package config

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/alecthomas/kong"

	"github.com/vooon/nebula-mnemosina/internal/model"
)

type CLI struct {
	DatabaseURL             string        `name:"database-url" env:"NEBULA_MNEMOSINA_DATABASE_URL" required:"" help:"PostgreSQL connection URL."`
	DatabaseEnableTimescale bool          `name:"database-enable-timescale" env:"NEBULA_MNEMOSINA_DATABASE_ENABLE_TIMESCALE" help:"Apply optional TimescaleDB hypertable migration."`
	Migrate                 bool          `name:"migrate" env:"NEBULA_MNEMOSINA_MIGRATE" default:"true" help:"Run embedded database migrations on startup."`
	RefreshViews            bool          `name:"refresh-views" env:"NEBULA_MNEMOSINA_REFRESH_VIEWS" default:"true" help:"Refresh Grafana materialized views after each successful poll write."`
	DataDir                 string        `name:"data-dir" env:"NEBULA_MNEMOSINA_DATA_DIR" default:"/data" help:"Directory for runtime data such as learned SSH host keys."`
	Lighthouses             []string      `name:"lighthouse" env:"NEBULA_MNEMOSINA_LIGHTHOUSES" sep:"," required:"" help:"Lighthouse SSH target. Format: name=user@host:port or user@host:port."`
	PollInterval            time.Duration `name:"poll-interval" env:"NEBULA_MNEMOSINA_POLL_INTERVAL" default:"30s" help:"Interval between polling rounds."`
	PollTimeout             time.Duration `name:"poll-timeout" env:"NEBULA_MNEMOSINA_POLL_TIMEOUT" default:"10s" help:"Timeout for one lighthouse polling run."`
	PollJitter              time.Duration `name:"poll-jitter" env:"NEBULA_MNEMOSINA_POLL_JITTER" default:"5s" help:"Maximum random delay added per lighthouse in a round."`
	Once                    bool          `name:"once" env:"NEBULA_MNEMOSINA_ONCE" help:"Run one polling round and exit."`
	LogLevel                string        `name:"log-level" env:"NEBULA_MNEMOSINA_LOG_LEVEL" default:"info" enum:"debug,info,warn,error" help:"Log level."`

	SSH  SSHConfig  `embed:"" prefix:"ssh-"`
	HTTP HTTPConfig `embed:"" prefix:"http-"`
	OTEL OTELConfig `embed:"" prefix:"otel-"`
}

type SSHConfig struct {
	KeyFile        string        `name:"key-file" env:"NEBULA_MNEMOSINA_SSH_KEY_FILE" help:"Path to the private key used for SSH authentication."`
	PrivateKey     string        `name:"private-key" env:"NEBULA_MNEMOSINA_SSH_PRIVATE_KEY" help:"Private key contents used for SSH authentication."`
	KeyPassphrase  string        `name:"key-passphrase" env:"NEBULA_MNEMOSINA_SSH_KEY_PASSPHRASE" help:"Optional private key passphrase."`
	KnownHostsPath string        `name:"known-hosts-path" env:"NEBULA_MNEMOSINA_SSH_KNOWN_HOSTS_PATH" default:"/data/known_hosts" help:"Path to known_hosts file. With accept-new, unknown keys are appended here."`
	HostKeyMode    string        `name:"host-key-mode" env:"NEBULA_MNEMOSINA_SSH_HOST_KEY_MODE" default:"accept-new" enum:"strict,accept-new,insecure" help:"SSH host key verification mode."`
	Timeout        time.Duration `name:"timeout" env:"NEBULA_MNEMOSINA_SSH_TIMEOUT" default:"10s" help:"SSH dial and command timeout."`
}

type HTTPConfig struct {
	Enabled bool   `name:"enabled" env:"NEBULA_MNEMOSINA_HTTP_ENABLED" default:"true" help:"Enable health and metrics HTTP server."`
	Address string `name:"address" env:"NEBULA_MNEMOSINA_HTTP_ADDRESS" default:":8080" help:"HTTP listen address."`
}

type OTELConfig struct {
	Enabled     bool    `name:"enabled" env:"NEBULA_MNEMOSINA_OTEL_ENABLED" help:"Enable OpenTelemetry tracing."`
	ServiceName string  `name:"service-name" env:"NEBULA_MNEMOSINA_OTEL_SERVICE_NAME" default:"nebula-mnemosina" help:"OTEL service name."`
	Endpoint    string  `name:"endpoint" env:"NEBULA_MNEMOSINA_OTEL_ENDPOINT" default:"localhost:4318" help:"OTLP HTTP endpoint host:port or URL."`
	SampleRatio float64 `name:"sample-ratio" env:"NEBULA_MNEMOSINA_OTEL_SAMPLE_RATIO" default:"1.0" help:"Trace sampling ratio from 0.0 to 1.0."`
}

type Config struct {
	CLI
	LighthouseTargets []model.Lighthouse
}

func Parse(args []string) (Config, error) {
	var cli CLI
	parser, err := kong.New(&cli,
		kong.Name("nebula-mnemosina"),
		kong.Description("Collect Nebula SSH state into PostgreSQL for Grafana."),
		kong.UsageOnError(),
	)
	if err != nil {
		return Config{}, err
	}
	_, err = parser.Parse(args)
	if err != nil {
		return Config{}, err
	}

	lighthouses, err := parseLighthouses(cli.Lighthouses)
	if err != nil {
		return Config{}, err
	}
	if cli.SSH.KeyFile == "" && cli.SSH.PrivateKey == "" {
		return Config{}, fmt.Errorf("either --ssh-key-file or NEBULA_MNEMOSINA_SSH_PRIVATE_KEY must be provided")
	}
	if cli.OTEL.SampleRatio < 0 || cli.OTEL.SampleRatio > 1 {
		return Config{}, fmt.Errorf("--otel-sample-ratio must be between 0.0 and 1.0")
	}

	return Config{
		CLI:               cli,
		LighthouseTargets: lighthouses,
	}, nil
}

func parseLighthouses(values []string) ([]model.Lighthouse, error) {
	targets := make([]model.Lighthouse, 0, len(values))
	seen := map[string]struct{}{}

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		name := ""
		target := value
		if before, after, ok := strings.Cut(value, "="); ok {
			name = strings.TrimSpace(before)
			target = strings.TrimSpace(after)
		}

		user, addr, ok := strings.Cut(target, "@")
		if !ok || user == "" || addr == "" {
			return nil, fmt.Errorf("invalid lighthouse %q: expected name=user@host:port or user@host:port", value)
		}

		host, port, err := splitHostPortDefault(addr, "4222")
		if err != nil {
			return nil, fmt.Errorf("invalid lighthouse %q: %w", value, err)
		}
		normalizedAddr := net.JoinHostPort(host, port)
		if name == "" {
			name = host
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate lighthouse name %q", name)
		}
		seen[name] = struct{}{}

		targets = append(targets, model.Lighthouse{
			Name:    name,
			User:    user,
			Address: normalizedAddr,
		})
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one lighthouse must be configured")
	}
	return targets, nil
}

func splitHostPortDefault(value, defaultPort string) (string, string, error) {
	host, port, err := net.SplitHostPort(value)
	if err == nil {
		return strings.Trim(host, "[]"), port, nil
	}
	if strings.Contains(value, ":") && strings.Count(value, ":") > 1 {
		return "", "", fmt.Errorf("IPv6 targets must use [host]:port syntax")
	}
	return value, defaultPort, nil
}
