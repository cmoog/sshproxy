// Package sshutil provides higher-level SSH features built
// atop the `golang.org/x/crypto/ssh` package.
package sshutil // import "cmoog.io/sshutil"

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"golang.org/x/crypto/ssh"
)

// ReverseProxy is an SSH Handler that takes an incoming request and sends it to another server,
// proxying the response back to the client.
type ReverseProxy struct {
	TargetHostname     string
	TargetClientConfig *ssh.ClientConfig

	// ErrorLog specifies an optional logger for errors
	// that occur when attempting to proxy.
	// If nil, logging is done via the log package's standard logger.
	ErrorLog *log.Logger
}

// NewSingleHostReverseProxy constructs a new *ReverseProxy instance.
func NewSingleHostReverseProxy(targetHost string, clientConfig *ssh.ClientConfig) *ReverseProxy {
	return &ReverseProxy{
		TargetHostname:     targetHost,
		TargetClientConfig: clientConfig,
	}
}

// Serve executes the reverse proxy between the sepcified target client hostname and the server connection.
func (r *ReverseProxy) Serve(ctx context.Context, serverConn net.Conn, serverConfig *ssh.ServerConfig) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger := r.ErrorLog
	if logger == nil {
		logger = log.New(os.Stderr, "", 0)
	}

	// TODO: do we need to make "network" an argument?
	targetConn, err := net.DialTimeout("tcp", r.TargetHostname, r.TargetClientConfig.Timeout)
	if err != nil {
		return fmt.Errorf("dial reverse proxy target: %w", err)
	}
	defer targetConn.Close()

	destConn, destChans, destReqs, err := ssh.NewClientConn(targetConn, r.TargetHostname, r.TargetClientConfig)
	if err != nil {
		return fmt.Errorf("new ssh client conn: %w", err)
	}

	sshServerConn, serverChans, serverReqs, err := ssh.NewServerConn(serverConn, serverConfig)
	if err != nil {
		return fmt.Errorf("accept ssh server conn: %w", err)
	}

	shutdownErr := make(chan error, 1)
	go func() {
		shutdownErr <- sshServerConn.Conn.Wait()
	}()

	go processChannels(ctx, destConn, serverChans, logger)
	go processChannels(ctx, sshServerConn.Conn, destChans, logger)
	go processRequests(ctx, destConn, serverReqs, logger)
	go processRequests(ctx, sshServerConn.Conn, destReqs, logger)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-shutdownErr:
		return err
	}
}

// processChannels handles each ssh.NewChannel concurrently.
func processChannels(ctx context.Context, destConn ssh.Conn, chans <-chan ssh.NewChannel, logger *log.Logger) {
	defer destConn.Close()
	for newCh := range chans {
		newCh := newCh
		go func() {
			err := handleChannel(ctx, destConn, newCh, logger)
			if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
				logger.Printf("sshutil: ReverseProxy handle channel error: %v", err)
			}
		}()
	}
}

// processRequests handles each *ssh.Request in series.
func processRequests(ctx context.Context, dest requestDest, requests <-chan *ssh.Request, logger *log.Logger) {
	for req := range requests {
		req := req
		err := handleRequest(ctx, dest, req)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
			logger.Printf("sshutil: ReverseProxy handle request error: %v", err)
		}
	}
}

// handleChannel performs the bicopy between the destination SSH connection and a new incoming channel.
func handleChannel(ctx context.Context, destConn ssh.Conn, newChannel ssh.NewChannel, logger *log.Logger) error {
	destCh, destReqs, err := destConn.OpenChannel(newChannel.ChannelType(), newChannel.ExtraData())
	if err != nil {
		if openChanErr, ok := err.(*ssh.OpenChannelError); ok {
			_ = newChannel.Reject(openChanErr.Reason, openChanErr.Message)
		} else {
			_ = newChannel.Reject(ssh.ConnectionFailed, err.Error())
		}
		return fmt.Errorf("open channel: %w", err)
	}
	defer destCh.Close()

	originCh, originRequests, err := newChannel.Accept()
	if err != nil {
		return fmt.Errorf("accept new channel: %w", err)
	}
	defer originCh.Close()

	// TODO(@cmoog) verify that this blocking behavior is correct.
	// As is, only one requests channel must be fully processed
	// before the ssh.Channels themselves are closed.
	requestsDone := make(chan struct{})
	go func() {
		defer close(requestsDone)
		processRequests(ctx, channelRequestDest{originCh}, destReqs, logger)
	}()

	go func() {
		// TODO(@cmoog) Verify: from limited testing, this request channel does not appear to be closed
		// by the client causing this function to hang if we wait on it.
		processRequests(ctx, channelRequestDest{destCh}, originRequests, logger)
	}()

	if err := bicopy(ctx, originCh, destCh); err != nil {
		return fmt.Errorf("bidirectional copy: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-requestsDone:
		return nil
	}
}

// bicopy copies data between the two channels,
// but does not perform complete closure.
// It will block until the context is cancelled or one of the
// copies has completed.
func bicopy(ctx context.Context, c1, c2 ssh.Channel) error {
	ctx1, cancel := context.WithCancel(ctx)
	defer cancel()

	copyWithCloseWrite := func(a, b ssh.Channel) {
		defer cancel()
		defer func() { _ = a.CloseWrite() }()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = io.Copy(a, b)
		}()
		_, _ = io.Copy(a.Stderr(), b.Stderr())
		wg.Wait()
	}

	go copyWithCloseWrite(c1, c2)
	go copyWithCloseWrite(c2, c1)

	<-ctx1.Done()

	// ignore Copy and CloseWrite errors, only error if parent context is done
	return ctx.Err()
}

// channelRequestDest wraps the ssh.Channel type to conform with the standard SendRequest function signiture.
type channelRequestDest struct {
	ssh.Channel
}

func (c channelRequestDest) SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error) {
	ok, err := c.Channel.SendRequest(name, wantReply, payload)
	return ok, nil, err
}

// requestDest defines a resource capable of receiving requests, (global or channel).
type requestDest interface {
	SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error)
}

func handleRequest(ctx context.Context, dest requestDest, request *ssh.Request) error {
	ok, payload, err := dest.SendRequest(request.Type, request.WantReply, request.Payload)
	if err != nil {
		if request.WantReply {
			if err := request.Reply(ok, payload); err != nil {
				return fmt.Errorf("reply after send failure: %w", err)
			}
		}
		return fmt.Errorf("send request: %w", err)
	}

	if request.WantReply {
		if err := request.Reply(ok, payload); err != nil {
			return fmt.Errorf("reply: %w", err)
		}
	}
	return nil
}
