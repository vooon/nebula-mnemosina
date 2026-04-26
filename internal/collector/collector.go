package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/vooon/nebula-mnemosina/internal/config"
	"github.com/vooon/nebula-mnemosina/internal/metrics"
	"github.com/vooon/nebula-mnemosina/internal/model"
	"github.com/vooon/nebula-mnemosina/internal/nebula"
)

type Runner interface {
	Run(ctx context.Context, lighthouse model.Lighthouse, command string) model.CommandResult
}

type Store interface {
	SavePollResult(ctx context.Context, result model.PollResult) error
	RefreshGrafanaViews(ctx context.Context) error
}

type Collector struct {
	cfg    config.Config
	runner Runner
	store  Store
	logger *slog.Logger
	tracer trace.Tracer
}

func New(cfg config.Config, runner Runner, store Store, logger *slog.Logger) *Collector {
	return &Collector{
		cfg:    cfg,
		runner: runner,
		store:  store,
		logger: logger,
		tracer: otel.Tracer("github.com/vooon/nebula-mnemosina/internal/collector"),
	}
}

func (c *Collector) Run(ctx context.Context) error {
	if c.cfg.Once {
		return c.runRound(ctx)
	}

	if err := c.runRound(ctx); err != nil {
		c.logger.Error("poll round failed", "error", err)
	}

	ticker := time.NewTicker(c.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.runRound(ctx); err != nil {
				c.logger.Error("poll round failed", "error", err)
			}
		}
	}
}

func (c *Collector) runRound(ctx context.Context) error {
	ctx, span := c.tracer.Start(ctx, "collector.poll_round")
	defer span.End()

	startedAt := time.Now()
	results := make(chan model.PollResult, len(c.cfg.LighthouseTargets))
	var wg sync.WaitGroup

	for _, lighthouse := range c.cfg.LighthouseTargets {
		lighthouse := lighthouse
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.cfg.PollJitter > 0 {
				delay := time.Duration(rand.Int63n(int64(c.cfg.PollJitter)))
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}

			pollCtx, cancel := context.WithTimeout(ctx, c.cfg.PollTimeout)
			defer cancel()
			results <- c.pollLighthouse(pollCtx, lighthouse)
		}()
	}

	wg.Wait()
	close(results)

	var errs []error
	saved := 0
	for result := range results {
		metrics.ObservePoll(result)
		if err := c.store.SavePollResult(ctx, result); err != nil {
			errs = append(errs, fmt.Errorf("save %s poll: %w", result.Lighthouse.Name, err))
			c.logger.Error("failed to save poll result", "lighthouse", result.Lighthouse.Name, "error", err)
			continue
		}
		saved++
		c.logger.Info("poll saved",
			"lighthouse", result.Lighthouse.Name,
			"success", result.Success(),
			"duration", result.Duration(),
			"commands", len(result.Commands),
		)
	}

	if saved > 0 && c.cfg.RefreshViews {
		if err := c.store.RefreshGrafanaViews(ctx); err != nil {
			errs = append(errs, fmt.Errorf("refresh grafana views: %w", err))
			c.logger.Error("failed to refresh grafana views", "error", err)
		}
	}

	span.SetAttributes(
		attribute.Int("lighthouse.count", len(c.cfg.LighthouseTargets)),
		attribute.Int("poll.saved", saved),
		attribute.Int64("poll.round_duration_ms", time.Since(startedAt).Milliseconds()),
	)
	if len(errs) > 0 {
		err := errors.Join(errs...)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (c *Collector) pollLighthouse(ctx context.Context, lighthouse model.Lighthouse) (result model.PollResult) {
	ctx, span := c.tracer.Start(ctx, "collector.poll_lighthouse",
		trace.WithAttributes(
			attribute.String("lighthouse.name", lighthouse.Name),
			attribute.String("ssh.target", lighthouse.Target()),
		),
	)
	defer span.End()

	result = model.PollResult{
		Lighthouse: lighthouse,
		StartedAt:  time.Now(),
	}
	defer func() {
		result.FinishedAt = time.Now()
		if result.Err != nil {
			span.RecordError(result.Err)
			span.SetStatus(codes.Error, result.Err.Error())
		}
	}()

	var errs []error
	run := func(command string) (model.CommandResult, bool) {
		commandCtx, commandSpan := c.tracer.Start(ctx, "collector.ssh_command",
			trace.WithAttributes(
				attribute.String("lighthouse.name", lighthouse.Name),
				attribute.String("ssh.command", command),
			),
		)
		commandResult := c.runner.Run(commandCtx, lighthouse, command)
		if commandResult.Err != nil {
			commandSpan.RecordError(commandResult.Err)
			commandSpan.SetStatus(codes.Error, commandResult.Err.Error())
		}
		commandSpan.End()

		result.Commands = append(result.Commands, commandResult)
		if commandResult.Err != nil {
			errs = append(errs, commandResult.Err)
			return commandResult, false
		}
		return commandResult, true
	}

	if command, ok := run("version"); ok {
		result.NebulaVersion = nebula.ParseVersion(command.Output)
	}
	if command, ok := run("device-info -json"); ok {
		device, err := nebula.ParseDeviceInfo(command.Output)
		if err != nil {
			errs = append(errs, err)
		} else {
			result.Device = device
		}
	}
	if command, ok := run("list-hostmap -json"); ok {
		hostmap, err := nebula.ParseHostmap(command.Output)
		if err != nil {
			errs = append(errs, err)
		} else {
			result.Hostmap = hostmap
		}
	}
	if command, ok := run("list-lighthouse-addrmap -json"); ok {
		addrmap, err := nebula.ParseLighthouseAddrmap(command.Output)
		if err != nil {
			errs = append(errs, err)
		} else {
			result.Addrmap = addrmap
		}
	}
	if command, ok := run("list-pending-hostmap -json"); ok {
		pending, err := nebula.ParseHostmap(command.Output)
		if err != nil {
			errs = append(errs, err)
		} else {
			result.Pending = pending
		}
	}
	if command, ok := run("print-relays"); ok {
		relays, err := nebula.ParseRelays(command.Output)
		if err != nil {
			errs = append(errs, err)
		} else {
			result.Relays = relays
		}
	}

	if len(errs) > 0 {
		result.Err = errors.Join(errs...)
	}
	return result
}
