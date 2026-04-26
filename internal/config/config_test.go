package config

import (
	"os"
	"testing"
)

func TestParseLighthouses(t *testing.T) {
	targets, err := parseLighthouses([]string{
		"lh1=nebula@192.168.110.1:4222",
		"nebula@192.168.110.2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Name != "lh1" || targets[0].User != "nebula" || targets[0].Address != "192.168.110.1:4222" {
		t.Fatalf("unexpected first target: %+v", targets[0])
	}
	if targets[1].Name != "192.168.110.2" || targets[1].Address != "192.168.110.2:4222" {
		t.Fatalf("unexpected default target: %+v", targets[1])
	}
}

func TestParseLighthousesRejectsDuplicateNames(t *testing.T) {
	_, err := parseLighthouses([]string{
		"lh=nebula@192.168.110.1:4222",
		"lh=nebula@192.168.110.2:4222",
	})
	if err == nil {
		t.Fatalf("expected duplicate name error")
	}
}

func TestParseRequiresDatabaseURL(t *testing.T) {
	unsetEnv(t, "MNEMO_DATABASE_URL")

	_, err := Parse([]string{"--lighthouse", "lh=nebula@192.168.110.1:4222", "--ssh-key-file", "key"}, "test")
	if err == nil {
		t.Fatalf("expected missing database URL error")
	}
}

func TestParseUsesShortEnvPrefix(t *testing.T) {
	t.Setenv("MNEMO_DATABASE_URL", "postgres://example")
	t.Setenv("MNEMO_LIGHTHOUSES", "lh=nebula@192.168.110.1:4222")
	t.Setenv("MNEMO_SSH_KEY_FILE", "/run/secrets/key")
	t.Setenv("MNEMO_DATABASE_ENABLE_TIMESCALE", "true")
	t.Setenv("MNEMO_HTTP_ADDRESS", ":9090")
	t.Setenv("MNEMO_PROMETHEUS_SD_PORT", "4281")

	cfg, err := Parse(nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://example" {
		t.Fatalf("unexpected database URL: %q", cfg.DatabaseURL)
	}
	if !cfg.DatabaseEnableTimescale {
		t.Fatalf("expected Timescale option from env")
	}
	if cfg.HTTP.Address != ":9090" {
		t.Fatalf("unexpected HTTP address: %q", cfg.HTTP.Address)
	}
	if cfg.PrometheusSD.Port != 4281 {
		t.Fatalf("unexpected Prometheus service discovery port: %d", cfg.PrometheusSD.Port)
	}
	if got := cfg.LighthouseTargets[0].Target(); got != "nebula@192.168.110.1:4222" {
		t.Fatalf("unexpected lighthouse target: %q", got)
	}
}

func TestParsePrometheusSDDefaults(t *testing.T) {
	t.Setenv("MNEMO_DATABASE_URL", "postgres://example")
	t.Setenv("MNEMO_LIGHTHOUSES", "lh=nebula@192.168.110.1:4222")
	t.Setenv("MNEMO_SSH_KEY_FILE", "/run/secrets/key")

	cfg, err := Parse(nil, "test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PrometheusSD.Port != 4280 {
		t.Fatalf("unexpected Prometheus service discovery port: %d", cfg.PrometheusSD.Port)
	}
	if cfg.PrometheusSD.MetricsPath != "/metrics" {
		t.Fatalf("unexpected Prometheus service discovery metrics path: %q", cfg.PrometheusSD.MetricsPath)
	}
}

func TestParseDebugShortcut(t *testing.T) {
	cfg, err := Parse([]string{
		"--database-url", "postgres://example",
		"--lighthouse", "lh=nebula@192.168.110.1:4222",
		"--ssh-key-file", "/run/secrets/key",
		"--debug",
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Debug {
		t.Fatalf("expected debug flag to be set")
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected debug flag to force log level debug, got %q", cfg.LogLevel)
	}
}

func TestParseNegatableBooleans(t *testing.T) {
	cfg, err := Parse([]string{
		"--database-url", "postgres://example",
		"--lighthouse", "lh=nebula@192.168.110.1:4222",
		"--ssh-key-file", "/run/secrets/key",
		"--no-migrate",
		"--no-refresh-views",
		"--no-http-enabled",
		"--no-otel-enabled",
		"--no-once",
		"--no-debug",
		"--no-database-enable-timescale",
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Migrate {
		t.Fatalf("expected migrate to be disabled")
	}
	if cfg.RefreshViews {
		t.Fatalf("expected refresh views to be disabled")
	}
	if cfg.HTTP.Enabled {
		t.Fatalf("expected HTTP to be disabled")
	}
	if cfg.OTEL.Enabled {
		t.Fatalf("expected OTEL to be disabled")
	}
	if cfg.Once {
		t.Fatalf("expected once to be disabled")
	}
	if cfg.DatabaseEnableTimescale {
		t.Fatalf("expected Timescale migration to be disabled")
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	oldValue, hadOldValue := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !hadOldValue {
			_ = os.Unsetenv(key)
			return
		}
		_ = os.Setenv(key, oldValue)
	})
}
