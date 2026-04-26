package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vooon/nebula-mnemosina/internal/collector"
	"github.com/vooon/nebula-mnemosina/internal/config"
	"github.com/vooon/nebula-mnemosina/internal/httpserver"
	"github.com/vooon/nebula-mnemosina/internal/migrate"
	"github.com/vooon/nebula-mnemosina/internal/sshclient"
	"github.com/vooon/nebula-mnemosina/internal/storage"
	"github.com/vooon/nebula-mnemosina/internal/telemetry"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := config.Parse(args)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel(cfg.LogLevel),
	}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := telemetry.Init(ctx, cfg.OTEL)
	if err != nil {
		return fmt.Errorf("initialize telemetry: %w", err)
	}
	defer func() {
		if err := shutdownTelemetry(context.Background()); err != nil {
			logger.Error("telemetry shutdown failed", "error", err)
		}
	}()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	if cfg.Migrate {
		logger.Info("running database migrations", "timescale", cfg.DatabaseEnableTimescale)
		if err := migrate.Run(ctx, pool, migrate.Options{EnableTimescale: cfg.DatabaseEnableTimescale}); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
	}

	store := storage.New(pool)
	if cfg.HTTP.Enabled {
		httpserver.Start(ctx, cfg.HTTP, store, logger)
	}

	sshRunner, err := sshclient.New(cfg.SSH)
	if err != nil {
		return fmt.Errorf("initialize ssh client: %w", err)
	}

	service := collector.New(cfg, sshRunner, store, logger)
	err = service.Run(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func slogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
