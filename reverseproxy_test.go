package sshutil

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

var (
	addr     = flag.String("ssh-addr", "", "specify a target address to dial SSH")
	user     = flag.String("ssh-user", "", "specify the SSH user with which to dial")
	password = flag.String("ssh-passwd", "", "specify the password with which to dial")
)

func Test_reverseProxy(t *testing.T) {
	t.Parallel()
	if *addr == "" || *user == "" || *password == "" {
		t.Fatalf("-ssh-addr, -ssh-user, and -ssh-passwd are all required flags")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	left, right, err := tcpPipeWithDialer(net.Dial, net.Listen)
	if err != nil {
		t.Fatalf("new net pipe: %v", err)
	}

	clientConfig := &ssh.ClientConfig{
		User:            *user,
		Auth:            []ssh.AuthMethod{ssh.Password(*password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer left.Close()
		clientSSHConn, clientChans, clientReqs, err := ssh.NewClientConn(left, "localhost", clientConfig)
		if err != nil {
			t.Errorf("new client conn: %v", err)
		}
		client := ssh.NewClient(clientSSHConn, clientChans, clientReqs)
		testSSHClient(t, client)
	}()

	serverConfig := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	signer, err := generateSigner()
	if err != nil {
		t.Fatalf("generate signer: %v", err)
	}
	serverConfig.AddHostKey(signer)

	serverConn, serverChans, serverReqs, err := ssh.NewServerConn(right, serverConfig)
	if err != nil {
		t.Fatalf("accept server conn: %v", err)
	}

	proxy := NewSingleHostReverseProxy(*addr, clientConfig)
	proxy.ErrorLog = log.Default()
	err = proxy.Serve(ctx, serverConn, serverChans, serverReqs)
	if err == nil {
		t.Fatalf("expected error from reverse proxy, got: %v", err)
	}
	wg.Wait()
}

func generateSigner() (ssh.Signer, error) {
	const blockType = "EC PRIVATE KEY"
	pkey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate rsa private key: %w", err)
	}

	byt, err := x509.MarshalECPrivateKey(pkey)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	pb := pem.Block{
		Type:    blockType,
		Headers: nil,
		Bytes:   byt,
	}
	p, err := ssh.ParsePrivateKey(pem.EncodeToMemory(&pb))
	if err != nil {
		return nil, err
	}
	return p, nil
}
