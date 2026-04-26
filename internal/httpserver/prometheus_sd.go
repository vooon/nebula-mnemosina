package httpserver

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/vooon/nebula-mnemosina/internal/config"
	"github.com/vooon/nebula-mnemosina/internal/model"
)

const prometheusSDPath = "/prometheus-sd"

type prometheusTargetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels,omitempty"`
}

func prometheusSDHandler(cfg config.PrometheusSDConfig, lighthouses []model.Lighthouse, logger *slog.Logger) http.Handler {
	groups, err := prometheusTargetGroups(cfg, lighthouses)
	if err != nil {
		logger.Error("failed to build prometheus service discovery targets", "error", err)
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodHead {
			return
		}
		if err := json.NewEncoder(w).Encode(groups); err != nil {
			logger.Error("failed to encode prometheus service discovery targets", "error", err)
		}
	})
}

func prometheusTargetGroups(cfg config.PrometheusSDConfig, lighthouses []model.Lighthouse) ([]prometheusTargetGroup, error) {
	groups := make([]prometheusTargetGroup, 0, len(lighthouses))
	port := strconv.Itoa(cfg.Port)

	for _, lighthouse := range lighthouses {
		host, _, err := net.SplitHostPort(lighthouse.Address)
		if err != nil {
			return nil, fmt.Errorf("split lighthouse %s address %q: %w", lighthouse.Name, lighthouse.Address, err)
		}

		groups = append(groups, prometheusTargetGroup{
			Targets: []string{net.JoinHostPort(host, port)},
			Labels: map[string]string{
				"__metrics_path__": cfg.MetricsPath,
				"lighthouse":       lighthouse.Name,
			},
		})
	}

	return groups, nil
}
