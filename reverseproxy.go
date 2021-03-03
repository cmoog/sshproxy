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
func (r *ReverseProxy) Serve(ctx context.Context, serverConn *ssh.ServerConn, serverChans <-chan ssh.NewChannel, serverReqs <-chan *ssh.Request) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var logger logger = r.ErrorLog
	if logger == nil {
		logger = defaultLogger{}
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

	shutdownErr := make(chan error, 1)
	go func() {
		shutdownErr <- serverConn.Conn.Wait()
	}()

	go processChannels(ctx, destConn, serverChans, logger)
	go processChannels(ctx, serverConn.Conn, destChans, logger)
	go processRequests(ctx, destConn, serverReqs, logger)
	go processRequests(ctx, serverConn.Conn, destReqs, logger)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-shutdownErr:
		return err
	}
}

type defaultLogger struct{}

// wrap the default logger
func (d defaultLogger) Printf(format string, v ...interface{}) { log.Printf(format, v...) }

type logger interface {
	Printf(format string, v ...interface{})
}

// processChannels handles each ssh.NewChannel concurrently.
func processChannels(ctx context.Context, destConn ssh.Conn, chans <-chan ssh.NewChannel, logger logger) {
	defer destConn.Close()
	for newCh := range chans {
		// reset the var scope for each goroutine
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
func processRequests(ctx context.Context, dest requestDest, requests <-chan *ssh.Request, logger logger) {
	for req := range requests {
		req := req
		err := handleRequest(ctx, dest, req)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
			logger.Printf("sshutil: ReverseProxy handle request error: %v", err)
		}
	}
}

// handleChannel performs the bicopy between the destination SSH connection and a new incoming channel.
func handleChannel(ctx context.Context, destConn ssh.Conn, newChannel ssh.NewChannel, logger logger) error {
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

	destRequestsDone := make(chan struct{})
	go func() {
		defer close(destRequestsDone)
		processRequests(ctx, channelRequestDest{originCh}, destReqs, logger)
	}()

	// This request channel does not get closed
	// by the client causing this function to hang if we wait on it.
	go processRequests(ctx, channelRequestDest{destCh}, originRequests, logger)

	if err := bicopy(ctx, originCh, destCh, logger); err != nil {
		return fmt.Errorf("channel bidirectional copy: %w", err)
	}

	select {
	case <-destRequestsDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// bicopy copies data between the two channels,
// but does not perform complete closure.
// It will block until the context is cancelled or the `alpha` channel
// has completed writing its data. Writes from the `beta` channel are not
// waited on.
func bicopy(ctx context.Context, alpha, beta ssh.Channel, logger logger) error {
	copyWithCloseWrite := func(a, b ssh.Channel) {
		defer a.CloseWrite()

		copyDone := make(chan struct{})
		go func() {
			defer close(copyDone)
			_, err := io.Copy(a, b)
			if err != nil && !errors.Is(err, io.EOF) {
				logger.Printf("sshutil: bicopy channel: %v", err)
			}
		}()
		_, err := io.Copy(a.Stderr(), b.Stderr())
		if err != nil && !errors.Is(err, io.EOF) {
			logger.Printf("sshutil: bicopy channel: %v", err)
		}
		<-copyDone
	}

	alphaWriteDone := make(chan struct{})
	go func() {
		defer close(alphaWriteDone)
		copyWithCloseWrite(alpha, beta)
	}()
	go copyWithCloseWrite(beta, alpha)

	select {
	case <-alphaWriteDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
