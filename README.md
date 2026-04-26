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
podman compose up --build
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

Kong derives environment variable names from flags with the `MNEMO_` prefix.
For example, `--ssh-key-file` maps to `MNEMO_SSH_KEY_FILE`.

```text
MNEMO_DATABASE_URL=postgres://nebula_mnemosina:nebula_mnemosina@postgres:5432/nebula_mnemosina?sslmode=disable
MNEMO_LIGHTHOUSES=lh1=nebula@192.168.110.1:4222,lh2=nebula@192.168.110.2:4222,lh3=nebula@192.168.110.3:4222
MNEMO_SSH_KEY_FILE=/run/secrets/nebula_ssh_key
MNEMO_SSH_PRIVATE_KEY=
MNEMO_SSH_KNOWN_HOSTS_PATH=/data/known_hosts
MNEMO_SSH_HOST_KEY_MODE=accept-new
MNEMO_POLL_INTERVAL=30s
MNEMO_LOG_LEVEL=info
MNEMO_DEBUG=false
MNEMO_OTEL_ENABLED=false
MNEMO_OTEL_ENDPOINT=otel-collector:4318
```

SSH host key modes:

```text
strict      require a known_hosts entry
accept-new  learn unknown keys, reject changed keys
insecure    skip verification
```

`accept-new` is the container-friendly default.

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

When `MNEMO_DATABASE_ENABLE_TIMESCALE=true`, the optional TimescaleDB migration
set is applied between the core tables and Grafana views.

## HTTP

The collector exposes:

```text
/healthz
/readyz
/metrics
/prometheus-sd
```

Prometheus metrics describe the collector itself. PostgreSQL/TimescaleDB is the
source of truth for Nebula topology history.

`/prometheus-sd` returns Prometheus HTTP service discovery target groups for
present peers from the latest successful lighthouse hostmap polls. Configured
lighthouses are also included as bootstrap targets when they are not already in
the present peer set. The Nebula stats port defaults to `4280`; override it with
`--prometheus-sd-port` or `MNEMO_PROMETHEUS_SD_PORT` if your `stats.listen` uses
a different port.

The HTTP SD `targets` value remains the real scrape address, while Nebula
identity is exposed as `__meta_nebula_*` labels for relabeling. For example,
map `__meta_nebula_name` to `instance` and `alias` if you want dashboards to
show Nebula node names instead of `ip:port`.

Example Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: nebula
    http_sd_configs:
      - url: http://nebula-mnemosina:12142/prometheus-sd
    relabel_configs:
      - source_labels: [__meta_nebula_name]
        target_label: instance
      - source_labels: [__meta_nebula_name]
        target_label: alias
      - source_labels: [__meta_nebula_vpn_addr]
        target_label: nebula_vpn_addr
      - source_labels: [__meta_nebula_cert_name]
        target_label: nebula_cert_name
      - source_labels: [__meta_nebula_source]
        target_label: nebula_sd_source
```

## Development

```bash
make generate
make test
make build
```

## E2E

The opt-in end-to-end test runs PostgreSQL, two Nebula lighthouse pods, three
Nebula peer pods, and `nebula-mnemosina` in Kubernetes. It is modeled after the
pathosd k3d workflow and uses generated disposable Nebula PKI/SSH fixtures.

```bash
make e2e
```

That path creates a disposable k3d/k3s cluster. To use the cluster from the
current kubeconfig, point `E2E_IMAGE` at an image that cluster can pull:

```bash
E2E_IMAGE=registry.example/nebula-mnemosina:e2e make e2e-current
```

If the target registry is already configured, `make e2e-current-push` builds,
pushes, deploys, and tests against the current kubeconfig.
For mutable tags, add `E2E_IMAGE_PULL_POLICY=Always`.

For a local Talos cluster, you can run a disposable registry inside the E2E
namespace and push the test image to it:

```bash
make e2e-current-registry
```

This exposes the registry as `E2E_REGISTRY_HOST:E2E_REGISTRY_NODE_PORT`, where
`E2E_REGISTRY_HOST` defaults to the first Kubernetes node internal IP and
`E2E_REGISTRY_NODE_PORT` defaults to `30500`. Talos still needs node-side
registry configuration for this plain HTTP endpoint, for example:

```yaml
machine:
  registries:
    mirrors:
      "192.168.61.61:30500":
        endpoints:
          - "http://192.168.61.61:30500"
```

If you have zoneomatic managing a private DNS zone, the registry can instead
use a real DNS name and Let's Encrypt certificate. This avoids Talos
insecure-registry configuration:

```bash
E2E_REGISTRY_DNS_NAME=registry.example.net \
E2E_ZONEOMATIC_URL=https://nsapi.example.com \
E2E_ZONEOMATIC_USER=... \
E2E_ZONEOMATIC_PASSWORD=... \
make e2e-current-registry-https
```

That registers `E2E_REGISTRY_DNS_NAME` with zoneomatic, creates an E2E
cert-manager Issuer backed by zoneomatic's acme-dns compatible `/acme` endpoint,
waits for the certificate, and pushes to:

```text
${E2E_REGISTRY_DNS_NAME}/nebula-mnemosina:e2e
```

On tenant-isolated clusters, the ingress controller may only be able to reach
backends in its own namespace. In that case, put the temporary registry,
Issuer, Certificate, and Ingress in the ingress namespace:

```bash
E2E_REGISTRY_NAMESPACE=tenant-root make e2e-current-registry-https
```

If the LoadBalancer IP advertised by the ingress service is not reachable from
your workstation, but the ingress HTTPS NodePort is reachable on a node IP, set
the registry port. The script will use a node that has an ingress controller
endpoint:

```bash
E2E_REGISTRY_PORT=30400 \
make e2e-current-registry-https
```

Set `E2E_REGISTRY_INGRESS_IP` only when you need to force the DNS target. For
NodePort with `ExternalTrafficPolicy: Local`, that IP must be a node running an
ingress controller pod.

Defaults:

```text
E2E_REGISTRY_NAMESPACE=nebula-mnemosina-e2e
E2E_REGISTRY_PORT=443
E2E_REGISTRY_DNS_TIMEOUT=300
E2E_REGISTRY_INGRESS_CLASS=tenant-root
E2E_REGISTRY_INGRESS_SERVICE_NAMESPACE=tenant-root
E2E_REGISTRY_INGRESS_SERVICE=root-ingress-controller
E2E_ACME_SERVER=https://acme-v02.api.letsencrypt.org/directory
```

Step-by-step k3d flow:

```bash
make e2e-cluster
make e2e-build
make e2e-deploy
make e2e-test
```

For the edit/retry loop against an existing cluster, use `make e2e-redeploy`.

`make e2e-fixtures` generates files under `tests/e2e/generated/`; that
directory is ignored by git.

Releases are handled by GoReleaser on `v*` tags and publish multi-arch
container images to GHCR.
