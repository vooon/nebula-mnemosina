package httpserver

import (
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
		{Name: "lh1", Address: "192.168.110.1:4222"},
		{Name: "lh6", Address: "[fd42:a3b0:110::1]:4222"},
	})
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
	if got := groups[0].Labels["lighthouse"]; got != "lh1" {
		t.Fatalf("unexpected lighthouse label: %q", got)
	}
	if got := groups[0].Labels["__metrics_path__"]; got != "/metrics" {
		t.Fatalf("unexpected metrics path label: %q", got)
	}
}

func TestPrometheusSDHandler(t *testing.T) {
	handler := prometheusSDHandler(config.PrometheusSDConfig{
		Port:        4280,
		MetricsPath: "/metrics",
	}, []model.Lighthouse{
		{Name: "lh1", Address: "192.168.110.1:4222"},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, prometheusSDPath, nil))

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
	if len(groups) != 1 || groups[0].Targets[0] != "192.168.110.1:4280" {
		t.Fatalf("unexpected target groups: %+v", groups)
	}
}
