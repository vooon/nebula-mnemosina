# nebula-mnemosina

Keep memory about Nebula network shapes.

`nebula-mnemosina` periodically connects to one or more Nebula SSH admin
endpoints, stores raw and normalized state in PostgreSQL/TimescaleDB, and
refreshes Grafana-facing materialized views.

## What It Polls

The first collector version runs these Nebula SSH commands on every configured
lighthouse:

```text
version
device-info -json
list-hostmap -json
list-lighthouse-addrmap -json
list-pending-hostmap -json
print-relays
```

Nebula SSH is accessed with Go's native SSH implementation. Only key
authentication is supported.

## Quick Start

Create an SSH key secret for the Nebula admin endpoint:

```bash
mkdir -p secrets
cp ~/.ssh/your_nebula_admin_key secrets/nebula_ssh_key
chmod 0600 secrets/nebula_ssh_key
```

Start the collector and TimescaleDB:

```bash
docker compose up --build
```

The compose stack uses one application data volume:

```text
nebula-mnemosina-data
```

By default, learned SSH host keys are stored at:

```text
/data/known_hosts
```

That means first connect uses TOFU behavior: unknown keys are accepted and
persisted, but changed keys fail future polls. Drop the
`nebula-mnemosina-data` volume if you intentionally want to relearn host keys.

## Configuration

Common environment variables:

```text
NEBULA_MNEMOSINA_DATABASE_URL=postgres://nebula_mnemosina:nebula_mnemosina@postgres:5432/nebula_mnemosina?sslmode=disable
NEBULA_MNEMOSINA_LIGHTHOUSES=lh1=nebula@192.168.110.1:4222,lh2=nebula@192.168.110.2:4222,lh3=nebula@192.168.110.3:4222
NEBULA_MNEMOSINA_SSH_KEY_FILE=/run/secrets/nebula_ssh_key
NEBULA_MNEMOSINA_SSH_PRIVATE_KEY=
NEBULA_MNEMOSINA_SSH_KNOWN_HOSTS_PATH=/data/known_hosts
NEBULA_MNEMOSINA_SSH_HOST_KEY_MODE=accept-new
NEBULA_MNEMOSINA_POLL_INTERVAL=30s
NEBULA_MNEMOSINA_OTEL_ENABLED=false
NEBULA_MNEMOSINA_OTEL_ENDPOINT=otel-collector:4318
```

SSH host key modes:

```text
strict      require a known_hosts entry
accept-new  learn unknown keys, reject changed keys
insecure    skip verification
```

`accept-new` is the Docker-friendly default.

## Database

The service runs embedded Tern migrations by default. Migrations are split into
three sets so TimescaleDB can stay optional:

```text
db/migrations/core
db/migrations/timescale
db/migrations/views
```

It stores:

- raw SSH command payloads
- poll run health
- normalized hostmap entries
- normalized lighthouse addrmap entries
- relay snapshots

Grafana should query materialized views instead of raw tables:

```text
mnemo_current_peers
mnemo_current_lighthouse_addrmap
mnemo_lighthouse_disagreement
mnemo_poll_health_5m
mnemo_peer_cert_inventory
```

When `NEBULA_MNEMOSINA_DATABASE_ENABLE_TIMESCALE=true`, the optional
TimescaleDB migration set is applied between the core tables and Grafana views.

## HTTP

The collector exposes:

```text
/healthz
/readyz
/metrics
```

Prometheus metrics describe the collector itself. PostgreSQL/TimescaleDB is the
source of truth for Nebula topology history.

## Development

```bash
make generate
make test
make build
```

Releases are handled by GoReleaser on `v*` tags and publish multi-arch Docker
images to GHCR.
