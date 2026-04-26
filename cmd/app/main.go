package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/perunio/perunio-facturador/internal/awssecrets"
	"github.com/perunio/perunio-facturador/internal/config"
	facturadorCrypto "github.com/perunio/perunio-facturador/internal/crypto"
	"github.com/perunio/perunio-facturador/internal/db"
	facturadorhttp "github.com/perunio/perunio-facturador/internal/http"
	"github.com/perunio/perunio-facturador/internal/r2"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(log); err != nil {
		log.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Secrets first — every later component depends on them.
	secrets := awssecrets.New()
	if err := secrets.Initialize(ctx); err != nil {
		return fmt.Errorf("initialize awssecrets: %w", err)
	}
	log.Info("secrets loaded", "source", secretsSource(secrets))

	// Decode the AES key now so we fail fast if it's malformed and so the rest
	// of the service can take []byte directly without re-decoding on every call.
	encKey, err := facturadorCrypto.DecodeKeyHex(secrets.EncryptionKey())
	if err != nil {
		return fmt.Errorf("decode encryption key: %w", err)
	}
	cfg.EncryptionKey = encKey

	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	log.Info("database connected")

	r2Client, err := r2.New(ctx, r2.Config{
		AccountID:       cfg.R2AccountID,
		AccessKeyID:     cfg.R2AccessKeyID,
		SecretAccessKey: cfg.R2SecretAccessKey,
		DocumentsBucket: cfg.R2DocumentsBucket,
	})
	if err != nil {
		return fmt.Errorf("init R2 client: %w", err)
	}
	log.Info("r2 client ready",
		"docs_bucket", cfg.R2DocumentsBucket,
	)

	srv := facturadorhttp.NewServer(facturadorhttp.Deps{
		Config:   cfg,
		Log:      log,
		Secrets:  secrets,
		Pool:     pool,
		R2:       r2Client,
	})

	httpServer := &http.Server{
		Addr:         net.JoinHostPort("", cfg.Port),
		Handler:      srv,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting server", "port", cfg.Port)
		errCh <- httpServer.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		log.Info("shutting down", "signal", sig)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	return httpServer.Shutdown(shutdownCtx)
}

func secretsSource(s *awssecrets.Service) string {
	if s.IsUsingAWS() {
		return "aws-secrets-manager"
	}
	return "env"
}
