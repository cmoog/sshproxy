# sshutil

`github.com/cmoog/sshutil` provides higher-level SSH features built
atop the `golang.org/x/crypto/ssh` package.

## `ReverseProxy`

`sshutil.ReverseProxy` implements a single host reverse proxy
for SSH servers and clients. Its API is modeled after the ergonomics
of the HTTP reverse proxy implementation.

```golang
err = httputil.NewSingleHostReverseProxy(targetURL).ServeHTTP(w, r)

err = sshutil.NewSingleHostReverseProxy(targetHost, &clientConfig).Serve(ctx, conn, &serverConfig)
```
