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
// Run this example on your local machine, with "username" and "password"
// substituted properly. This will allow you to dial port 2222 and be reverse
// proxied through to your OpenSSH server on port 22.
//
// Run this server in the backround, then dial
//
//   $ ssh -p2222 localhost
//

const exampleUsername = "username"
const examplePassword = "password"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverConfig := ssh.ServerConfig{
		NoClientAuth: true,
	}
	signer, err := generateSigner()
	if err != nil {
		log.Fatal(err)
	}
	serverConfig.AddHostKey(signer)

	l, err := net.Listen("tcp", "localhost:2222")
	if err != nil {
		log.Fatal(err)
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			defer conn.Close()
			serverConn, serverChans, serverReqs, err := ssh.NewServerConn(conn, &serverConfig)
			if err != nil {
				log.Println(err)
				return
			}
			rp := sshutil.NewSingleHostReverseProxy("localhost:22", &ssh.ClientConfig{
				User:            exampleUsername,
				Auth:            []ssh.AuthMethod{ssh.Password(examplePassword)},
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				Timeout:         3 * time.Second,
			})
			err = rp.Serve(ctx, serverConn, serverChans, serverReqs)
			if err != nil {
				log.Println(err)
				return
			}
		}()
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
