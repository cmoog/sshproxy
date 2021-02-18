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

err = sshutil.NewSingleHostReverseProxy(targetHost, &clientConfig).Serve(ctx, conn, &serverConfig)
```
