# sshproxy

[![Documentation](https://godoc.org/cmoog.io/sshproxy?status.svg)](https://pkg.go.dev/cmoog.io/sshproxy)
[![Go Report Card](https://goreportcard.com/badge/cmoog.io/sshproxy)](https://goreportcard.com/report/cmoog.io/sshproxy)
[![codecov](https://codecov.io/gh/cmoog/sshproxy/branch/master/graph/badge.svg?token=IQ87G7H7OA)](https://codecov.io/gh/cmoog/sshproxy)

Package sshproxy provides a slim SSH reverse proxy built
atop the `golang.org/x/crypto/ssh` package.

```text
go get cmoog.io/sshproxy
```

## Authorization termination proxy

`sshproxy.ReverseProxy` implements a single host reverse proxy
for SSH servers and clients. Its API is modeled after the ergonomics
of the [HTTP reverse proxy](https://pkg.go.dev/net/http/httputil#ReverseProxy) implementation
from the standard library.

It enables the proxy to perform authorization termination,
whereby custom authorization logic of the single entrypoint can protect
a set of SSH hosts hidden in a private network.

For example, one could conceivably use OAuth as a basis for verifying
identity and ownership of public keys.

## Example usage

Consider the following bare-bones example with error handling omitted for brevity.

```go
package main

import (
  "net"
  "golang.org/x/crypto/ssh"
  "cmoog.io/sshproxy"
)

func main() {
  serverConfig := ssh.ServerConfig{
    // TODO: add your custom public key authentication logic
    PublicKeyCallback: customPublicKeyAuthenticationLogic
  }
  serverConfig.AddHostKey(reverseProxyHostKey)

  listener, _ := net.Listen("tcp", reverseProxyEntrypoint)
  for {
    clientConnection, _ := listener.Accept()
    go func() {
      defer clientConnection.Close()
      sshConn, sshChannels, sshRequests, _ := ssh.NewServerConn(clientConnection, &serverConfig)

      // TODO: add your custom routing logic based the SSH `user` string, and/or the public key
      targetServer, targetServerConnectionConfig := customRoutingLogic(sshConn.User())

      proxy := sshproxy.New(targetServer, targetServerConnectionConfig)
      _ = proxy.Serve(ctx, sshConn, sshChannels, sshRequests)
    }()
  }
}
```
