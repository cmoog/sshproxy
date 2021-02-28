package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net"
	"time"

	"cmoog.io/sshutil"
	"golang.org/x/crypto/ssh"
)

// The following example demonstrates a simple usage of sshutil.ReverseProxy.
//
// Run this example on your local machine, with "your_username" and "your_password"
// substituted properly. This will allow you to dial port 2222 and be reverse
// proxied through to your OpenSSH server on port 22.

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := net.Listen("tcp", "localhost:2222")
	if err != nil {
		log.Fatal(err)
	}
	conn, err := listener.Accept()
	if err != nil {
		log.Fatal(err)
	}
	serverConfig := ssh.ServerConfig{
		NoClientAuth: true,
	}
	signer, err := generateSigner()
	if err != nil {
		log.Fatal(err)
	}
	serverConfig.AddHostKey(signer)

	const targetHost = "localhost:22"
	clientConfig := ssh.ClientConfig{
		User: "your_username",
		// password auth and public key auth work just as you'd expect, for simpliciy, we'll use
		// password auth in this example
		Auth:            []ssh.AuthMethod{ssh.Password("your_password")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	}
	serverConn, serverChans, serverReqs, err := ssh.NewServerConn(conn, &serverConfig)
	if err != nil {
		log.Fatal(err)
	}

	err = sshutil.NewSingleHostReverseProxy(targetHost, &clientConfig).Serve(ctx, serverConn, serverChans, serverReqs)
	if err != nil {
		log.Fatal(err)
	}
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
