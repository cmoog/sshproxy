# sshutil

`github.com/cmoog/sshutil` provides higher-level SSH features built
atop the `golang.org/x/crypto/ssh` package.

## `ReverseProxy`

`sshutil.ReverseProxy` implements a single host reverse proxy
for SSH servers and clients. Its API is modeled after the ergonomics
of the HTTP reverse proxy implementation.

```golang
err = httputil.NewSingleHostReverseProxy(targetURL).ServeHTTP(w, r)

err = sshutil.ReverseProxy(ctx, sshutil.ReverseProxyConfig{
  ServerConn:         serverConn,
  ServerChannels:     serverChannels,
  ServerRequests:     serverRequests,
  TargetConn:         targetConn,
  TargetHostname:     targetHost,
  TargetClientConfig: &clientConfig,
})
```
