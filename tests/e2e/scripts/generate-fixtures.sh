#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
out_dir="${repo_root}/tests/e2e/generated"
nebula_cert_version="${NEBULA_CERT_VERSION:-v1.10.3}"
nodes=(lh1 lh2 peer1 peer2 peer3)

declare -A vpn_ips=(
  [lh1]="192.168.111.101"
  [lh2]="192.168.111.102"
  [peer1]="192.168.111.103"
  [peer2]="192.168.111.104"
  [peer3]="192.168.111.105"
)

declare -A groups=(
  [lh1]="lighthouse,router"
  [lh2]="lighthouse,router"
  [peer1]="peer"
  [peer2]="peer"
  [peer3]="peer"
)

mkdir -p "${out_dir}"
rm -f "${out_dir}"/*

nebula_cert() {
  if command -v nebula-cert >/dev/null 2>&1; then
    nebula-cert "$@"
    return
  fi

  go run "github.com/slackhq/nebula/cmd/nebula-cert@${nebula_cert_version}" "$@"
}

(
  cd "${out_dir}"

  nebula_cert ca -name "nebula-mnemosina-e2e"
  if [[ -f ca.cert && ! -f ca.crt ]]; then
    mv ca.cert ca.crt
  fi

  for node in "${nodes[@]}"; do
    nebula_cert sign -name "${node}" -ip "${vpn_ips[${node}]}/24" -groups "${groups[${node}]}"
    if [[ -f "${node}.cert" && ! -f "${node}.crt" ]]; then
      mv "${node}.cert" "${node}.crt"
    fi
  done

  ssh-keygen -t ed25519 -N "" -f ssh_client_key -C "nebula-mnemosina-e2e" >/dev/null
  ssh-keygen -t ed25519 -N "" -f ssh_host_ed25519_key -C "nebula-mnemosina-e2e-host" >/dev/null
)

ssh_pub="$(cat "${out_dir}/ssh_client_key.pub")"

write_common_config() {
  local node="$1"

  cat <<EOF
pki:
  ca: /config/ca.crt
  cert: /config/${node}.crt
  key: /config/${node}.key

listen:
  host: 0.0.0.0
  port: 4242

tun:
  disabled: true

stats:
  type: prometheus
  listen: 0.0.0.0:4280
  path: /metrics
  interval: 5s
  message_metrics: true
  lighthouse_metrics: true
  subsystem: nebula

firewall:
  outbound:
    - port: any
      proto: any
      host: any
  inbound:
    - port: any
      proto: any
      host: any
EOF
}

for node in lh1 lh2; do
  cat > "${out_dir}/${node}.yml" <<EOF
static_host_map: {}

lighthouse:
  am_lighthouse: true
  interval: 5

sshd:
  enabled: true
  listen: 0.0.0.0:4222
  host_key: /config/ssh_host_ed25519_key
  authorized_users:
    - user: nebula
      keys:
        - "${ssh_pub}"

$(write_common_config "${node}")
EOF
done

for node in peer1 peer2 peer3; do
  cat > "${out_dir}/${node}.yml" <<EOF
static_host_map:
  "${vpn_ips[lh1]}": ["nebula-lh1:4242"]
  "${vpn_ips[lh2]}": ["nebula-lh2:4242"]

lighthouse:
  am_lighthouse: false
  interval: 5
  hosts:
    - "${vpn_ips[lh1]}"
    - "${vpn_ips[lh2]}"

$(write_common_config "${node}")
EOF
done

chmod 0600 "${out_dir}"/*.key "${out_dir}/ssh_client_key" "${out_dir}/ssh_host_ed25519_key"
