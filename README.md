# sshutil

[![Documentation](https://godoc.org/cmoog.io/sshutil?status.svg)](https://pkg.go.dev/cmoog.io/sshutil)
[![Go Report Card](https://goreportcard.com/badge/cmoog.io/sshutil)](https://goreportcard.com/report/cmoog.io/sshutil)

`sshutil` provides higher-level SSH features in Go built
atop the `golang.org/x/crypto/ssh` package.

```text
go get cmoog.io/sshutil
```

## `ReverseProxy`

`sshutil.ReverseProxy` implements a single host reverse proxy
for SSH servers and clients. Its API is modeled after the ergonomics
of the HTTP reverse proxy implementation.

```golang
httputil.NewSingleHostReverseProxy(targetURL).ServeHTTP(w, r)

err = sshutil.NewSingleHostReverseProxy(targetHost, &clientConfig).Serve(ctx, conn, channels, requests)
```

## `Router`

`sshutil.Router` defines a simple interface for routing incoming SSH
connections to 1-N target servers. Modeled after the usability of
`http.Handler`, this interface cleanly wraps the underlying TCP listener
for the common case of serving an SSH reverse proxy server from a single host port.

```golang
type Router interface {
  Route(context.Context, *ssh.ServerConn) (targetAddr string, client *ssh.ClientConfig, err error)
}

func ServeProxy(ctx context.Context, router Router, addr string, serverConfig *ssh.ServerConfig) error
```
