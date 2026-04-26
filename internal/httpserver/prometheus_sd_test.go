package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vooon/nebula-mnemosina/internal/config"
	"github.com/vooon/nebula-mnemosina/internal/model"
)

func TestPrometheusTargetGroupsUseLighthouseHostsWithStatsPort(t *testing.T) {
	groups, err := prometheusTargetGroups(config.PrometheusSDConfig{
		Port:        4280,
		MetricsPath: "/metrics",
	}, []model.Lighthouse{
		{Name: "lh1", User: "nebula", Address: "192.168.110.1:4222"},
		{Name: "lh6", User: "nebula", Address: "[fd42:a3b0:110::1]:4222"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 target groups, got %d", len(groups))
	}

	if got := groups[0].Targets[0]; got != "192.168.110.1:4280" {
		t.Fatalf("unexpected IPv4 target: %q", got)
	}
	if got := groups[1].Targets[0]; got != "[fd42:a3b0:110::1]:4280" {
		t.Fatalf("unexpected IPv6 target: %q", got)
	}
	if got := groups[0].Labels["__metrics_path__"]; got != "/metrics" {
		t.Fatalf("unexpected metrics path label: %q", got)
	}
	if got := groups[0].Labels["__meta_nebula_source"]; got != "configured_lighthouse" {
		t.Fatalf("unexpected source label: %q", got)
	}
	if got := groups[0].Labels["__meta_nebula_name"]; got != "lh1" {
		t.Fatalf("unexpected name label: %q", got)
	}
	if got := groups[0].Labels["__meta_nebula_lighthouse_name"]; got != "lh1" {
		t.Fatalf("unexpected lighthouse name label: %q", got)
	}
	if got := groups[0].Labels["__meta_nebula_ssh_target"]; got != "nebula@192.168.110.1:4222" {
		t.Fatalf("unexpected ssh target label: %q", got)
	}
}

func TestPrometheusTargetGroupsPreferPresentPeers(t *testing.T) {
	groups, err := prometheusTargetGroups(config.PrometheusSDConfig{
		Port:        4280,
		MetricsPath: "/metrics",
	}, []model.Lighthouse{
		{Name: "lh1", User: "nebula", Address: "192.168.110.1:4222"},
	}, []model.PresentPeer{
		{
			LighthouseName:  "lh1",
			PeerKey:         "peer-20",
			PrimaryVPNAddr:  "192.168.110.20",
			VPNAddrs:        []string{"192.168.110.20", "fd42:a3b0:110::20"},
			CertName:        "plein-aire",
			CertFingerprint: "fingerprint-20",
			CertGroups:      []string{"router"},
			NebulaVersion:   "1.10.3",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected peer and lighthouse target groups, got %d", len(groups))
	}

	if got := groups[0].Targets[0]; got != "192.168.110.20:4280" {
		t.Fatalf("unexpected peer target: %q", got)
	}
	if got := groups[0].Labels["__meta_nebula_source"]; got != "present_peer" {
		t.Fatalf("unexpected source label: %q", got)
	}
	if got := groups[0].Labels["__meta_nebula_cert_name"]; got != "plein-aire" {
		t.Fatalf("unexpected cert name label: %q", got)
	}
	if got := groups[0].Labels["__meta_nebula_observed_by_lighthouse"]; got != "lh1" {
		t.Fatalf("unexpected observed-by label: %q", got)
	}
	if got := groups[0].Labels["__meta_nebula_name"]; got != "plein-aire" {
		t.Fatalf("unexpected name label: %q", got)
	}
	if got := groups[0].Labels["__meta_nebula_vpn_addrs"]; got != "192.168.110.20,fd42:a3b0:110::20" {
		t.Fatalf("unexpected vpn addrs label: %q", got)
	}
	if _, ok := groups[0].Labels["instance"]; ok {
		t.Fatalf("instance should be produced by relabeling, not service discovery")
	}
	if _, ok := groups[0].Labels["alias"]; ok {
		t.Fatalf("alias should be produced by relabeling, not service discovery")
	}
}

func TestPrometheusSDHandler(t *testing.T) {
	store := &prometheusSDFakeStore{
		peers: []model.PresentPeer{
			{
				LighthouseName: "lh1",
				PeerKey:        "peer-20",
				PrimaryVPNAddr: "192.168.110.20",
				CertName:       "plein-aire",
			},
		},
	}
	handler := prometheusSDHandler(config.PrometheusSDConfig{
		Port:        4280,
		MetricsPath: "/metrics",
	}, []model.Lighthouse{
		{Name: "lh1", User: "nebula", Address: "192.168.110.1:4222"},
	}, store, slog.New(slog.NewTextHandler(io.Discard, nil)))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/prometheus-sd", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected content type: %q", got)
	}

	var groups []prometheusTargetGroup
	if err := json.Unmarshal(recorder.Body.Bytes(), &groups); err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Targets[0] != "192.168.110.20:4280" || groups[1].Targets[0] != "192.168.110.1:4280" {
		t.Fatalf("unexpected target groups: %+v", groups)
	}

	store.peers = []model.PresentPeer{{PrimaryVPNAddr: "192.168.110.40"}}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/prometheus-sd", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected second status: %d", recorder.Code)
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &groups); err != nil {
		t.Fatal(err)
	}
	if groups[0].Targets[0] != "192.168.110.40:4280" {
		t.Fatalf("handler did not reload peers on second request: %+v", groups)
	}
	if store.calls != 2 {
		t.Fatalf("expected two store calls, got %d", store.calls)
	}
}

type prometheusSDFakeStore struct {
	peers []model.PresentPeer
	calls int
}

func (s *prometheusSDFakeStore) Ping(context.Context) error {
	return nil
}

func (s *prometheusSDFakeStore) ListPresentHostmapPeers(context.Context) ([]model.PresentPeer, error) {
	s.calls++
	return s.peers, nil
}
