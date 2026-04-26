# AGENTS.md

## Scope

Instructions for contributors and agents working in this repository:
`nebula-mnemosina`, a Go service that periodically polls Nebula SSH admin
endpoints and stores topology history in PostgreSQL/TimescaleDB for Grafana.

## Architecture

- Entry point: `cmd/nebula-mnemosina`.
- Config: Kong CLI/env parsing with `MNEMO_` environment prefix.
- Logging: `charmbracelet/log` as the `slog` handler.
- Tracing: optional OpenTelemetry tracing.
- Metrics: Prometheus client metrics for the collector itself.
- Data collection: Go native SSH, key auth only, multiple lighthouses.
- Storage: PostgreSQL via pgx, migrations via Tern, query code via sqlc.
- Grafana surface: materialized views in `db/migrations/views`.
- Container runtime: scratch image with binary at `/nebula-mnemosina`.

## Project Layout

```text
cmd/nebula-mnemosina/   CLI entry point and process wiring
internal/config/        Kong config structs, parsing, validation
internal/collector/     Poll loop and Nebula command orchestration
internal/sshclient/     Native Go SSH client and known_hosts handling
internal/nebula/        Parsers for Nebula SSH JSON/text output
internal/storage/       PostgreSQL writes and query adapters
internal/httpserver/    /healthz, /readyz, /metrics, /prometheus-sd
internal/metrics/       Collector Prometheus metrics
internal/telemetry/     OpenTelemetry setup
internal/db/            sqlc-generated code; do not edit manually
db/queries/             sqlc SQL queries
db/migrations/          Tern migrations: core, optional timescale, views
docker/rootfs/data/     Seed directory copied into scratch images
```

## Generated Code And Migrations

- Do not hand-edit `internal/db/*`; edit `db/queries/*` or migrations and run
  `sqlc generate`.
- CI and release use sqlc `1.31.1`; keep generated headers and workflow pins in
  sync if sqlc is upgraded.
- Migrations are embedded and run by the service by default.
- TimescaleDB support is optional; keep core migrations valid for plain
  PostgreSQL.
- Grafana should use materialized views rather than raw tables. When changing
  schema or query semantics, check the views and README examples.

## Config Rules

- Use Kong struct tags consistently.
- Required values should use `required:""` where appropriate.
- Boolean CLI flags should use `negatable:""` so `--no-*` forms work.
- Default env prefix is `MNEMO`, not the longer project name.
- Lighthouse config accepts `name=user@host:port` or `user@host:port`; default
  SSH admin port is `4222`.
- SSH private key may come from file or env. Known hosts default to
  `/data/known_hosts` with `accept-new`.

## Nebula Data Handling

- The SSH admin API is the compatibility boundary. Keep parsers tolerant of
  Nebula output shape where feasible.
- Prefer local, small wire DTOs for SSH JSON output over importing the whole
  Nebula root package just to unmarshal responses, especially where Nebula uses
  interfaces such as certificates.
- Preserve raw command payloads and raw normalized entries where the code
  already stores them.
- `/prometheus-sd` should query current stored state on every request. Its
  `targets` values are real scrape addresses; Nebula identity should be exposed
  as `__meta_nebula_*` labels for relabeling.

## Container And Local Runtime

- Local container workflow uses Podman. Do not assume Docker is installed.
- `Makefile` defaults: `CONTAINER_TOOL ?= podman`,
  `COMPOSE ?= podman compose`.
- Runtime image is `scratch`; avoid adding CA bundles or shell assumptions unless
  the runtime actually needs them.
- No PostgreSQL SSL is used in the default compose setup.
- App HTTP default port is `12142`.
- Nebula stats service-discovery target port defaults to `4280`.
- Runtime data lives under `/data`; the compose data volume is
  `nebula-mnemosina-data`.

## Verification

Use focused checks while iterating and broader checks before finishing:

```bash
sqlc generate
git diff --exit-code -- internal/db
CGO_ENABLED=0 go test ./...
golangci-lint run ./...
go test -race -coverprofile=coverage.out ./...
goreleaser check
```

For container changes, verify with Podman when available:

```bash
podman build --progress=plain -t nebula-mnemosina:local .
```

## Editing Rules

- Keep patches targeted; do not revert unrelated user changes.
- Do not commit or fabricate credentials, private keys, `.env` files, or real
  secrets.
- Preserve user-provided examples unless the requested behavior requires
  updating them.
- Keep README, compose defaults, Dockerfiles, CI, and GoReleaser config aligned
  when changing ports, generated-tool versions, container paths, or release
  behavior.
- This repo's recent commit messages are short imperative subjects; follow that
  style unless the user asks for another convention.
