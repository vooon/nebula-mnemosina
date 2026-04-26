#!/usr/bin/env bash
set -euo pipefail

generated_dir="${E2E_GENERATED:-tests/e2e/generated}"
dns_name="${E2E_REGISTRY_DNS_NAME:-}"
timeout="${E2E_REGISTRY_DNS_TIMEOUT:-300}"
registry_env="${generated_dir}/registry.env"

if [[ -f "${registry_env}" ]]; then
  # shellcheck source=/dev/null
  source "${registry_env}"
fi

expected_ip="${E2E_REGISTRY_SELECTED_IP:-${E2E_REGISTRY_INGRESS_IP:-}}"

if [[ -z "${dns_name}" ]]; then
  echo "E2E_REGISTRY_DNS_NAME is required" >&2
  exit 1
fi

if [[ -z "${expected_ip}" ]]; then
  echo "E2E_REGISTRY_SELECTED_IP is required; run prepare-registry-dns.sh first" >&2
  exit 1
fi

resolve_ipv4() {
  getent ahostsv4 "${dns_name}" | awk '{print $1}' | sort -u
}

deadline=$((SECONDS + timeout))

while ((SECONDS <= deadline)); do
  resolved="$(resolve_ipv4 || true)"
  if grep -Fxq "${expected_ip}" <<<"${resolved}"; then
    echo "${dns_name} resolves to ${expected_ip}"
    exit 0
  fi

  if [[ -z "${resolved}" ]]; then
    echo "Waiting for ${dns_name} to resolve to ${expected_ip}; currently no A record"
  else
    echo "Waiting for ${dns_name} to resolve to ${expected_ip}; currently ${resolved//$'\n'/, }"
  fi
  sleep 5
done

echo "${dns_name} did not resolve to ${expected_ip} within ${timeout}s" >&2
exit 1
