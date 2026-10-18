// Command web-api serves the public member API on port 3333. It answers the
// routes ported from kaderisasi-web-be and proxies the rest to the Adonis
// service at WEB_BE_LEGACY_URL (see docs/WEB_API.md).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kaderisasi/web/internal/config"
	"kaderisasi/web/internal/database"
	"kaderisasi/web/internal/dbgen"
	"kaderisasi/web/internal/webapi"
)

func run() error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	logger, err := config.NewLogger(os.Stdout, c.LogLevel)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.Open(ctx, c)
	if err != nil {
		return err
	}
	defer pool.Close()
	app := &webapi.Server{Config: c, Users: dbgen.New(pool), Logger: logger}
	server := &http.Server{Addr: c.Address(), Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	failure := make(chan error, 1)
	go func() {
		logger.Info("web API listening", "address", c.Address(), "legacy", c.LegacyURL.String())
		failure <- server.ListenAndServe()
	}()
	select {
	case err := <-failure:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("web API stopped", "error", err.Error())
		os.Exit(1)
	}
}
