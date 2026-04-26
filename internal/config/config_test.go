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
	if got := cfg.LighthouseTargets[0].Target(); got != "nebula@192.168.110.1:4222" {
		t.Fatalf("unexpected lighthouse target: %q", got)
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
