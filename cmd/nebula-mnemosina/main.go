package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	charmLog "github.com/charmbracelet/log"
	"github.com/jackc/pgx/v5/pgxpool"
	promVersion "github.com/prometheus/common/version"

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
	cfg, err := config.Parse(args, versionText())
	if err != nil {
		return err
	}
	logLevel, err := charmLog.ParseLevel(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("parse log level: %w", err)
	}

	logger := slog.New(charmLog.NewWithOptions(os.Stdout, charmLog.Options{
		Level:           logLevel,
		ReportTimestamp: true,
		TimeFunction:    charmLog.NowUTC,
		TimeFormat:      time.RFC3339,
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

func versionText() string {
	if promVersion.Version == "" {
		promVersion.Version = "dev"
	}
	return promVersion.Print("nebula-mnemosina")
}
