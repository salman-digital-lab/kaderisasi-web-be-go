// Command web-api serves the public member API on port 3333, the Go
// replacement for kaderisasi-web-be (see docs/MIGRATION.md).
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
	"kaderisasi/web/internal/google"
	"kaderisasi/web/internal/js"
	"kaderisasi/web/internal/mail"
	"kaderisasi/web/internal/storage"
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
	js.Local = c.Location
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.Open(ctx, c)
	if err != nil {
		return err
	}
	defer pool.Close()
	app := &webapi.Server{Config: c, Users: dbgen.New(pool), Pool: pool, Logger: logger,
		Storage: storage.New(c.DriveEndpoint, c.DriveRegion, c.DriveKey, c.DriveSecret, c.DriveBucket),
		Mail:    mail.SMTP{Host: c.SMTPHost, Port: c.SMTPPort, Username: c.SMTPUsername, Password: c.SMTPPassword}}
	if c.GoogleClientID != "" && c.GoogleClientSecret != "" && c.GoogleRedirectURI != "" {
		app.Google = &google.Client{ClientID: c.GoogleClientID, ClientSecret: c.GoogleClientSecret, RedirectURI: c.GoogleRedirectURI}
	}
	server := &http.Server{Addr: c.Address(), Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	failure := make(chan error, 1)
	go func() {
		logger.Info("web API listening", "address", c.Address())
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
