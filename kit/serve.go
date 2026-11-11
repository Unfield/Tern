package kit

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ListenAndServe runs the app until it receives SIGINT or SIGTERM, then
// shuts down gracefully, letting in-flight requests finish for up to 10
// seconds.
//
// It listens on the address set with [WithAddr], or HOST:PORT from the
// environment, defaulting to port 3000 on all interfaces, or 443 when the
// App serves HTTPS (WithTLS, WithAutoTLS).
func (a *App) ListenAndServe() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return a.Serve(ctx)
}

// Serve runs the app until ctx is canceled, then shuts down gracefully.
func (a *App) Serve(ctx context.Context) error {
	raw, err := a.listen()
	if err != nil {
		return err
	}
	ln, err := a.WrapListener(raw)
	if err != nil {
		_ = raw.Close()
		return err
	}
	srv := &http.Server{
		Addr:              raw.Addr().String(),
		Handler:           a,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	a.ConfigureServer(srv)
	srv.RegisterOnShutdown(a.beginShutdown)
	if err := a.Start(); err != nil {
		_ = ln.Close()
		return err
	}
	scheme := "http://"
	if a.tls != nil {
		scheme = "https://"
	}
	if a.dev == nil { // under tern dev, the dev server prints its own banner
		a.logger.Info("tern: listening", "addr", scheme+displayAddr(ln.Addr()))
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	if t := a.tls; t != nil && t.redirect != nil {
		rctx, stopRedirects := context.WithCancel(context.Background())
		defer stopRedirects()
		rerr := make(chan error, 1)
		go serveRedirects(rctx, t, rerr)
		go func() {
			if err := <-rerr; err != nil {
				a.logger.Warn("tern: not serving plain HTTP; certificates come over port 443 alone", "addr", t.httpAddr, "error", err)
			}
		}()
	}

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	if a.dev == nil {
		a.logger.Info("tern: shutting down")
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer func() { _ = a.Close() }()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// listen opens the address the App serves on.
func (a *App) listen() (net.Listener, error) {
	addr := a.opts.addr
	if addr == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "3000"
			if a.opts.tls != nil || a.opts.autoTLS != nil || autoTLSFromEnv() != nil {
				port = "443"
			}
		}
		addr = net.JoinHostPort(os.Getenv("HOST"), port)
	}
	return net.Listen("tcp", addr)
}

func displayAddr(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		host = "localhost"
	}
	return net.JoinHostPort(host, port)
}
