package httpserver

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/vooon/nebula-mnemosina/internal/config"
	"github.com/vooon/nebula-mnemosina/internal/model"
)

type prometheusTargetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels,omitempty"`
}

func prometheusSDHandler(cfg config.PrometheusSDConfig, lighthouses []model.Lighthouse, store Store, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		peers, err := store.ListPresentHostmapPeers(r.Context())
		if err != nil {
			logger.Error("failed to list prometheus service discovery peers", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		groups, err := prometheusTargetGroups(cfg, lighthouses, peers)
		if err != nil {
			logger.Error("failed to build prometheus service discovery targets", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

func prometheusTargetGroups(cfg config.PrometheusSDConfig, lighthouses []model.Lighthouse, peers []model.PresentPeer) ([]prometheusTargetGroup, error) {
	groups := make([]prometheusTargetGroup, 0, len(lighthouses)+len(peers))
	port := strconv.Itoa(cfg.Port)
	seenTargets := map[string]struct{}{}

	for _, peer := range peers {
		if peer.PrimaryVPNAddr == "" {
			continue
		}

		target := net.JoinHostPort(peer.PrimaryVPNAddr, port)
		if _, ok := seenTargets[target]; ok {
			continue
		}
		seenTargets[target] = struct{}{}

		nodeName := peer.CertName
		if nodeName == "" {
			nodeName = peer.PrimaryVPNAddr
		}
		labels := map[string]string{
			"__metrics_path__":          cfg.MetricsPath,
			"__meta_nebula_address":     target,
			"__meta_nebula_name":        nodeName,
			"__meta_nebula_source":      "present_peer",
			"__meta_nebula_vpn_addr":    peer.PrimaryVPNAddr,
			"__meta_nebula_metric_path": cfg.MetricsPath,
		}
		setLabel(labels, "__meta_nebula_peer_key", peer.PeerKey)
		setLabel(labels, "__meta_nebula_vpn_addrs", strings.Join(peer.VPNAddrs, ","))
		setLabel(labels, "__meta_nebula_cert_name", peer.CertName)
		setLabel(labels, "__meta_nebula_cert_fingerprint", peer.CertFingerprint)
		setLabel(labels, "__meta_nebula_observed_by_lighthouse", peer.LighthouseName)
		setLabel(labels, "__meta_nebula_version", peer.NebulaVersion)
		if len(peer.CertGroups) > 0 {
			labels["__meta_nebula_cert_groups"] = strings.Join(peer.CertGroups, ",")
		}

		groups = append(groups, prometheusTargetGroup{
			Targets: []string{target},
			Labels:  labels,
		})
	}

	for _, lighthouse := range lighthouses {
		host, _, err := net.SplitHostPort(lighthouse.Address)
		if err != nil {
			return nil, fmt.Errorf("split lighthouse %s address %q: %w", lighthouse.Name, lighthouse.Address, err)
		}

		target := net.JoinHostPort(host, port)
		if _, ok := seenTargets[target]; ok {
			continue
		}
		seenTargets[target] = struct{}{}

		groups = append(groups, prometheusTargetGroup{
			Targets: []string{target},
			Labels: map[string]string{
				"__metrics_path__":                cfg.MetricsPath,
				"__meta_nebula_address":           target,
				"__meta_nebula_lighthouse_host":   host,
				"__meta_nebula_lighthouse_name":   lighthouse.Name,
				"__meta_nebula_lighthouse_user":   lighthouse.User,
				"__meta_nebula_metric_path":       cfg.MetricsPath,
				"__meta_nebula_name":              lighthouse.Name,
				"__meta_nebula_source":            "configured_lighthouse",
				"__meta_nebula_ssh_address":       lighthouse.Address,
				"__meta_nebula_ssh_target":        lighthouse.Target(),
				"__meta_nebula_stats_target_host": host,
			},
		})
	}

	return groups, nil
}

func setLabel(labels map[string]string, name, value string) {
	if value == "" {
		return
	}
	labels[name] = value
}
