# sshutil

[![Documentation](https://godoc.org/cmoog.io/sshutil?status.svg)](https://pkg.go.dev/cmoog.io/sshutil)
[![Go Report Card](https://goreportcard.com/badge/cmoog.io/sshutil)](https://goreportcard.com/report/cmoog.io/sshutil)
[![codecov](https://codecov.io/gh/cmoog/sshutil/branch/master/graph/badge.svg?token=IQ87G7H7OA)](https://codecov.io/gh/cmoog/sshutil)

`sshutil` provides higher-level SSH features in Go built
atop the `golang.org/x/crypto/ssh` package.

```text
go get cmoog.io/sshutil
```

## Authorization termination proxy with `ReverseProxy`

`sshutil.ReverseProxy` implements a single host reverse proxy
for SSH servers and clients. Its API is modeled after the ergonomics
of the HTTP reverse proxy implementation.

It enables the proxy to perform authorization termination,
whereby custom authorization logic of the single entrypoint can protect
a set of SSH hosts hidden in a private network.

For example, one could conceivably use OAuth as a basis for verifying
identity and ownership of public keys.

```golang
httputil.NewSingleHostReverseProxy(targetURL).ServeHTTP(w, r)

err = sshutil.NewSingleHostReverseProxy(targetHost, &clientConfig).Serve(ctx, conn, channels, requests)
```
