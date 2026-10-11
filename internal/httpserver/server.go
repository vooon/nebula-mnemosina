package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/vooon/nebula-mnemosina/internal/config"
	"github.com/vooon/nebula-mnemosina/internal/model"
)

type Store interface {
	Ping(ctx context.Context) error
	ListPresentHostmapPeers(ctx context.Context) ([]model.PresentPeer, error)
}

// Start serves HTTP endpoints. /readyz reports not ready until ready is set,
// so the server can start before migrations without passing readiness early.
func Start(ctx context.Context, cfg config.HTTPConfig, prometheusSD config.PrometheusSDConfig, lighthouses []model.Lighthouse, store Store, ready *atomic.Bool, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			http.Error(w, "starting", http.StatusServiceUnavailable)
			return
		}
		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := store.Ping(pingCtx); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/prometheus-sd", prometheusSDHandler(prometheusSD, lighthouses, store, logger))

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("http server listening", "address", cfg.Address)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server failed", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("http server shutdown failed", "error", err)
		}
	}()

	return server
}
