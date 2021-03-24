# sshproxy

[![Documentation](https://godoc.org/cmoog.io/sshproxy?status.svg)](https://pkg.go.dev/cmoog.io/sshproxy)
[![Go Report Card](https://goreportcard.com/badge/cmoog.io/sshproxy)](https://goreportcard.com/report/cmoog.io/sshproxy)
[![codecov](https://codecov.io/gh/cmoog/sshproxy/branch/master/graph/badge.svg?token=IQ87G7H7OA)](https://codecov.io/gh/cmoog/sshproxy)

Package sshproxy provides a slim SSH reverse proxy built
atop the `golang.org/x/crypto/ssh` package.

```text
go get cmoog.io/sshproxy
```

## Authorization termination proxy with `ReverseProxy`

`sshproxy.ReverseProxy` implements a single host reverse proxy
for SSH servers and clients. Its API is modeled after the ergonomics
of the HTTP reverse proxy implementation.

It enables the proxy to perform authorization termination,
whereby custom authorization logic of the single entrypoint can protect
a set of SSH hosts hidden in a private network.

For example, one could conceivably use OAuth as a basis for verifying
identity and ownership of public keys.

```golang
httputil.NewSingleHostReverseProxy(targetURL).ServeHTTP(w, r)

err = sshproxy.New(targetHost, &clientConfig).Serve(ctx, conn, channels, requests)
```
