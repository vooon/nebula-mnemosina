#!/usr/bin/env bash
set -euo pipefail

kubectl_bin="${KUBECTL:-kubectl}"
namespace="${E2E_NAMESPACE:-nebula-mnemosina-e2e}"
registry_namespace="${E2E_REGISTRY_NAMESPACE:-${namespace}}"
generated_dir="${E2E_GENERATED:-tests/e2e/generated}"
dns_name="${E2E_REGISTRY_DNS_NAME:-}"
registry_port="${E2E_REGISTRY_PORT:-443}"
ingress_class="${E2E_REGISTRY_INGRESS_CLASS:-tenant-root}"
ingress_service_namespace="${E2E_REGISTRY_INGRESS_SERVICE_NAMESPACE:-tenant-root}"
ingress_service="${E2E_REGISTRY_INGRESS_SERVICE:-root-ingress-controller}"
ingress_ip="${E2E_REGISTRY_INGRESS_IP:-}"
zoneomatic_url="${E2E_ZONEOMATIC_URL:-}"
zoneomatic_user="${E2E_ZONEOMATIC_USER:-}"
zoneomatic_password="${E2E_ZONEOMATIC_PASSWORD:-}"
zoneomatic_acmedns_host="${E2E_ZONEOMATIC_ACMEDNS_HOST:-}"
acme_server="${E2E_ACME_SERVER:-https://acme-v02.api.letsencrypt.org/directory}"

require() {
  local name="$1"
  local value="$2"

  if [[ -z "${value}" ]]; then
    echo "${name} is required" >&2
    exit 1
  fi
}

require E2E_ZONEOMATIC_URL "${zoneomatic_url}"
require E2E_ZONEOMATIC_USER "${zoneomatic_user}"
require E2E_ZONEOMATIC_PASSWORD "${zoneomatic_password}"
require E2E_REGISTRY_DNS_NAME "${dns_name}"

if [[ -z "${ingress_ip}" && "${registry_port}" != "443" ]]; then
  ingress_node="$(
    "${kubectl_bin}" -n "${ingress_service_namespace}" get endpointslice \
      -l "kubernetes.io/service-name=${ingress_service}" \
      -o jsonpath='{.items[0].endpoints[0].nodeName}' 2>/dev/null || true
  )"
  if [[ -z "${ingress_node}" ]]; then
    ingress_node="$(
      "${kubectl_bin}" -n "${ingress_service_namespace}" get endpoints "${ingress_service}" \
        -o jsonpath='{.subsets[0].addresses[0].nodeName}' 2>/dev/null || true
    )"
  fi
  if [[ -n "${ingress_node}" ]]; then
    ingress_ip="$(
      "${kubectl_bin}" get node "${ingress_node}" \
        -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}'
    )"
    echo "Using ingress endpoint node ${ingress_node} (${ingress_ip}) for registry port ${registry_port}"
  fi
fi

if [[ -z "${ingress_ip}" ]]; then
  ingress_ip="$(
    "${kubectl_bin}" -n "${ingress_service_namespace}" get service "${ingress_service}" \
      -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
  )"
fi
require E2E_REGISTRY_INGRESS_IP "${ingress_ip}"

if [[ -z "${zoneomatic_acmedns_host}" ]]; then
  zoneomatic_acmedns_host="${zoneomatic_url%/}/acme"
fi

if [[ "${registry_namespace}" != "${namespace}" ]] &&
  "${kubectl_bin}" get namespace "${namespace}" >/dev/null 2>&1; then
  "${kubectl_bin}" -n "${namespace}" delete ingress nebula-mnemosina-registry --ignore-not-found
fi

mkdir -p "${generated_dir}"
registry_env="${generated_dir}/registry.env"

{
  printf 'E2E_REGISTRY_SELECTED_IP=%s\n' "${ingress_ip}"
  printf 'E2E_REGISTRY_SELECTED_PORT=%s\n' "${registry_port}"
} >"${registry_env}"

echo "Updating ${dns_name} -> ${ingress_ip} in zoneomatic"
curl -fsS -u "${zoneomatic_user}:${zoneomatic_password}" \
  --get \
  --data-urlencode "hostname=${dns_name}" \
  --data-urlencode "myip=${ingress_ip}" \
  "${zoneomatic_url%/}/nic/update" >/dev/null

acmedns_json="${generated_dir}/registry-acmedns.json"
jq -n \
  --arg domain "${dns_name}" \
  --arg username "${zoneomatic_user}" \
  --arg password "${zoneomatic_password}" \
  --arg fulldomain "${dns_name}" \
  --arg subdomain "${dns_name}" \
  '{
    ($domain): {
      username: $username,
      password: $password,
      fulldomain: $fulldomain,
      subdomain: $subdomain,
      allowfrom: []
    }
  }' >"${acmedns_json}"

"${kubectl_bin}" -n "${registry_namespace}" create secret generic nebula-mnemosina-registry-acmedns \
  --from-file=acmedns.json="${acmedns_json}" \
  --dry-run=client \
  -o yaml |
  "${kubectl_bin}" apply -f -

cat <<EOF | "${kubectl_bin}" apply -f -
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: nebula-mnemosina-registry
  namespace: ${registry_namespace}
spec:
  acme:
    privateKeySecretRef:
      name: nebula-mnemosina-registry-acme-account
    server: ${acme_server}
    solvers:
      - selector:
          dnsNames:
            - ${dns_name}
        dns01:
          acmeDNS:
            host: ${zoneomatic_acmedns_host}
            accountSecretRef:
              name: nebula-mnemosina-registry-acmedns
              key: acmedns.json
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: nebula-mnemosina-registry
  namespace: ${registry_namespace}
spec:
  secretName: nebula-mnemosina-registry-tls
  issuerRef:
    name: nebula-mnemosina-registry
    kind: Issuer
    group: cert-manager.io
  dnsNames:
    - ${dns_name}
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: nebula-mnemosina-registry
  namespace: ${registry_namespace}
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "0"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "600"
spec:
  ingressClassName: ${ingress_class}
  tls:
    - hosts:
        - ${dns_name}
      secretName: nebula-mnemosina-registry-tls
  rules:
    - host: ${dns_name}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: nebula-mnemosina-registry
                port:
                  name: registry
EOF
