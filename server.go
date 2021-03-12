package sshutil

import (
	"context"
	"errors"
	"io"
	"log"
	"net"

	"golang.org/x/crypto/ssh"
)

// Router routes an SSH server connection to some target SSH endpoint.
type Router interface {
	Route(context.Context, *ssh.ServerConn) (targetAddr string, client *ssh.ClientConfig, err error)
}

// ServeProxy listens on the TCP network address addr and then calls
// the Router to route incoming SSH connections.
func ServeProxy(ctx context.Context, router Router, addr string, serverConfig *ssh.ServerConfig) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go func() { defer listener.Close(); <-ctx.Done() }()
	for {
		conn, err := listener.Accept()
		if err != nil && errors.Is(err, net.ErrClosed) {
			return err
		}
		go func() {
			err := handle(ctx, conn, router, serverConfig)
			if err != nil && !errors.Is(err, io.EOF) {
				log.Default().Printf("sshutil server error: %v", err)
			}
		}()
	}
}

func handle(ctx context.Context, conn net.Conn, router Router, serverConfig *ssh.ServerConfig) error {
	defer conn.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	serverConn, serverChans, serverRequests, err := ssh.NewServerConn(conn, serverConfig)
	if err != nil {
		return err
	}
	targetAddr, clientConfig, err := router.Route(ctx, serverConn)
	if err != nil {
		return err
	}
	rp := NewSingleHostReverseProxy(targetAddr, clientConfig)
	if err := rp.Serve(ctx, serverConn, serverChans, serverRequests); err != nil {
		return err
	}

	return nil
}
