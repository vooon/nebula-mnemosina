package retention

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/vooon/nebula-mnemosina/internal/config"
	"github.com/vooon/nebula-mnemosina/internal/metrics"
)

const batchSize = 1000

// Hypertables whose rows expire with their poll run.
var pollHypertables = []string{
	"hostmap_entries",
	"lighthouse_addrmap_entries",
	"relay_snapshots",
}

type Store interface {
	DropChunks(ctx context.Context, table string, cutoff time.Time) (int64, error)
	DeleteExpiredPollRuns(ctx context.Context, cutoff time.Time, batchSize int32) (int64, error)
	DeleteExpiredRawCommandPayloads(ctx context.Context, cutoff time.Time, batchSize int32) (int64, error)
}

type Pruner struct {
	cfg       config.RetentionConfig
	timescale bool
	store     Store
	logger    *slog.Logger
}

func New(cfg config.RetentionConfig, timescale bool, store Store, logger *slog.Logger) *Pruner {
	return &Pruner{
		cfg:       cfg,
		timescale: timescale,
		store:     store,
		logger:    logger,
	}
}

func (p *Pruner) Enabled() bool {
	return p.cfg.Period > 0 || p.cfg.RawPeriod > 0
}

// Run prunes immediately and then on every retention interval until ctx is done.
func (p *Pruner) Run(ctx context.Context) {
	if !p.Enabled() {
		p.logger.Info("retention disabled")
		return
	}

	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()

	for {
		if err := p.Prune(ctx, time.Now()); err != nil && !errors.Is(err, context.Canceled) {
			p.logger.Error("retention run failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Pruner) Prune(ctx context.Context, now time.Time) (err error) {
	defer func() {
		metrics.ObserveRetentionRun(err == nil)
	}()

	if p.cfg.RawPeriod > 0 {
		cutoff := now.Add(-p.cfg.RawPeriod).UTC()
		if p.timescale {
			if err := p.dropChunks(ctx, "raw_command_payloads", cutoff); err != nil {
				return err
			}
		}
		if err := p.deleteBatches(ctx, "raw_command_payloads", cutoff, p.store.DeleteExpiredRawCommandPayloads); err != nil {
			return err
		}
	}

	if p.cfg.Period > 0 {
		cutoff := now.Add(-p.cfg.Period).UTC()
		if p.timescale {
			// Dropping whole chunks first is cheap and returns disk space
			// immediately; the cascade below only sees the remainder.
			tables := pollHypertables
			if p.cfg.RawPeriod == 0 {
				tables = append([]string{"raw_command_payloads"}, tables...)
			}
			for _, table := range tables {
				if err := p.dropChunks(ctx, table, cutoff); err != nil {
					return err
				}
			}
		}
		if err := p.deleteBatches(ctx, "poll_runs", cutoff, p.store.DeleteExpiredPollRuns); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pruner) dropChunks(ctx context.Context, table string, cutoff time.Time) error {
	dropped, err := p.store.DropChunks(ctx, table, cutoff)
	if err != nil {
		return err
	}
	metrics.ObserveRetentionDroppedChunks(table, dropped)
	if dropped > 0 {
		p.logger.Info("retention dropped chunks", "table", table, "chunks", dropped, "cutoff", cutoff)
	}
	return nil
}

func (p *Pruner) deleteBatches(ctx context.Context, table string, cutoff time.Time, deleteBatch func(context.Context, time.Time, int32) (int64, error)) error {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		deleted, err := deleteBatch(ctx, cutoff, batchSize)
		if err != nil {
			return fmt.Errorf("prune %s: %w", table, err)
		}
		total += deleted
		metrics.ObserveRetentionDeletedRows(table, deleted)
		if deleted < batchSize {
			break
		}
	}
	if total > 0 {
		p.logger.Info("retention deleted rows", "table", table, "rows", total, "cutoff", cutoff)
	}
	return nil
}
