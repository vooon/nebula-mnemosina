GOCACHE ?= /tmp/nebula-mnemosina-gocache
CGO_ENABLED ?= 0
CONTAINER_TOOL ?= podman
CONTAINER_PUSH_FLAGS ?=
COMPOSE ?= podman compose
K3D ?= k3d
KUBECTL ?= kubectl

E2E_CLUSTER ?= nebula-mnemosina-e2e
E2E_IMAGE ?= nebula-mnemosina:e2e
E2E_IMAGE_PULL_POLICY ?= IfNotPresent
E2E_NAMESPACE ?= nebula-mnemosina-e2e
E2E_GENERATED ?= tests/e2e/generated
E2E_REGISTRY_NAMESPACE ?= $(E2E_NAMESPACE)
E2E_REGISTRY_NODE_PORT ?= 30500
E2E_REGISTRY_HOST ?=
E2E_REGISTRY_DNS_NAME ?=
E2E_REGISTRY_PORT ?= 443
E2E_REGISTRY_DNS_TIMEOUT ?= 300
E2E_REGISTRY_INGRESS_CLASS ?= tenant-root
E2E_REGISTRY_INGRESS_SERVICE_NAMESPACE ?= tenant-root
E2E_REGISTRY_INGRESS_SERVICE ?= root-ingress-controller
E2E_REGISTRY_INGRESS_IP ?=
E2E_ZONEOMATIC_URL ?=
E2E_ZONEOMATIC_ACMEDNS_HOST ?=
E2E_ACME_SERVER ?= https://acme-v02.api.letsencrypt.org/directory
export E2E_ACME_SERVER E2E_GENERATED E2E_NAMESPACE E2E_REGISTRY_DNS_NAME
export E2E_REGISTRY_DNS_TIMEOUT E2E_REGISTRY_INGRESS_CLASS E2E_REGISTRY_INGRESS_IP
export E2E_REGISTRY_INGRESS_SERVICE E2E_REGISTRY_INGRESS_SERVICE_NAMESPACE
export E2E_REGISTRY_NAMESPACE E2E_REGISTRY_PORT
export E2E_ZONEOMATIC_ACMEDNS_HOST E2E_ZONEOMATIC_PASSWORD E2E_ZONEOMATIC_URL E2E_ZONEOMATIC_USER
export KUBECTL

.PHONY: generate
generate:
	sqlc generate

.PHONY: test
test:
	CGO_ENABLED=$(CGO_ENABLED) GOCACHE=$(GOCACHE) go test ./...

.PHONY: build
build:
	CGO_ENABLED=$(CGO_ENABLED) GOCACHE=$(GOCACHE) go build -o bin/nebula-mnemosina ./cmd/nebula-mnemosina

.PHONY: tidy
tidy:
	GOCACHE=$(GOCACHE) go mod tidy

.PHONY: image-build
image-build:
	$(CONTAINER_TOOL) build -t nebula-mnemosina:local .

.PHONY: compose-up
compose-up:
	$(COMPOSE) up --build

.PHONY: e2e-fixtures
e2e-fixtures:
	tests/e2e/scripts/generate-fixtures.sh

.PHONY: e2e-cluster
e2e-cluster:
	$(K3D) cluster create $(E2E_CLUSTER) --wait

.PHONY: e2e-image-build
e2e-image-build:
	$(CONTAINER_TOOL) build -t $(E2E_IMAGE) .

.PHONY: e2e-image-push
e2e-image-push: e2e-image-build
	$(CONTAINER_TOOL) push $(CONTAINER_PUSH_FLAGS) $(E2E_IMAGE)

.PHONY: e2e-build
e2e-build: e2e-image-build
	mkdir -p $(E2E_GENERATED)
	$(CONTAINER_TOOL) save $(E2E_IMAGE) -o $(E2E_GENERATED)/nebula-mnemosina-image.tar
	$(K3D) image import $(E2E_GENERATED)/nebula-mnemosina-image.tar -c $(E2E_CLUSTER)

.PHONY: e2e-deploy
e2e-deploy: e2e-fixtures
	$(KUBECTL) apply -f tests/e2e/manifests/namespace.yaml
	$(KUBECTL) wait --for=jsonpath='{.status.phase}'=Active namespace/$(E2E_NAMESPACE) --timeout=60s
	$(KUBECTL) -n $(E2E_NAMESPACE) delete pod,service -l app=nebula --ignore-not-found
	$(KUBECTL) -n $(E2E_NAMESPACE) create secret generic nebula-pki --from-file=ca.crt=$(E2E_GENERATED)/ca.crt --from-file=lh1.crt=$(E2E_GENERATED)/lh1.crt --from-file=lh1.key=$(E2E_GENERATED)/lh1.key --from-file=lh2.crt=$(E2E_GENERATED)/lh2.crt --from-file=lh2.key=$(E2E_GENERATED)/lh2.key --from-file=peer1.crt=$(E2E_GENERATED)/peer1.crt --from-file=peer1.key=$(E2E_GENERATED)/peer1.key --from-file=peer2.crt=$(E2E_GENERATED)/peer2.crt --from-file=peer2.key=$(E2E_GENERATED)/peer2.key --from-file=peer3.crt=$(E2E_GENERATED)/peer3.crt --from-file=peer3.key=$(E2E_GENERATED)/peer3.key --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(KUBECTL) -n $(E2E_NAMESPACE) create secret generic nebula-ssh --from-file=ssh_client_key=$(E2E_GENERATED)/ssh_client_key --from-file=ssh_host_ed25519_key=$(E2E_GENERATED)/ssh_host_ed25519_key --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(KUBECTL) -n $(E2E_NAMESPACE) create configmap nebula-config --from-file=lh1.yml=$(E2E_GENERATED)/lh1.yml --from-file=lh2.yml=$(E2E_GENERATED)/lh2.yml --from-file=peer1.yml=$(E2E_GENERATED)/peer1.yml --from-file=peer2.yml=$(E2E_GENERATED)/peer2.yml --from-file=peer3.yml=$(E2E_GENERATED)/peer3.yml --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(KUBECTL) apply -f tests/e2e/manifests/postgres.yaml
	$(KUBECTL) apply -f tests/e2e/manifests/nebula.yaml
	$(KUBECTL) apply -f tests/e2e/manifests/nebula-mnemosina.yaml
	$(KUBECTL) -n $(E2E_NAMESPACE) set image deployment/nebula-mnemosina nebula-mnemosina=$(E2E_IMAGE)
	$(KUBECTL) -n $(E2E_NAMESPACE) patch deployment/nebula-mnemosina -p '{"spec":{"template":{"spec":{"containers":[{"name":"nebula-mnemosina","imagePullPolicy":"$(E2E_IMAGE_PULL_POLICY)"}]}}}}'
	$(KUBECTL) -n $(E2E_NAMESPACE) rollout restart deployment/nebula-mnemosina
	$(KUBECTL) -n $(E2E_NAMESPACE) rollout status deployment/nebula-mnemosina --timeout=120s
	$(KUBECTL) -n $(E2E_NAMESPACE) wait --for=condition=Ready pod -l app=postgres --timeout=120s
	$(KUBECTL) -n $(E2E_NAMESPACE) wait --for=condition=Ready pod -l app=nebula --timeout=120s
	$(KUBECTL) -n $(E2E_NAMESPACE) wait --for=condition=Ready pod -l app=nebula-mnemosina --timeout=120s

.PHONY: e2e-registry
e2e-registry:
	$(KUBECTL) get namespace $(E2E_REGISTRY_NAMESPACE) >/dev/null || $(KUBECTL) create namespace $(E2E_REGISTRY_NAMESPACE)
	$(KUBECTL) -n $(E2E_REGISTRY_NAMESPACE) apply -f tests/e2e/manifests/registry.yaml
	$(KUBECTL) -n $(E2E_REGISTRY_NAMESPACE) rollout status deployment/nebula-mnemosina-registry --timeout=120s

.PHONY: e2e-registry-nodeport
e2e-registry-nodeport: e2e-registry
	$(KUBECTL) -n $(E2E_REGISTRY_NAMESPACE) patch service/nebula-mnemosina-registry -p '{"spec":{"type":"NodePort","ports":[{"name":"registry","port":5000,"targetPort":"registry","nodePort":$(E2E_REGISTRY_NODE_PORT)}]}}'

.PHONY: e2e-registry-https
e2e-registry-https: e2e-registry
	tests/e2e/scripts/prepare-registry-dns.sh
	$(KUBECTL) -n $(E2E_REGISTRY_NAMESPACE) wait --for=condition=Ready certificate/nebula-mnemosina-registry --timeout=300s
	tests/e2e/scripts/wait-registry-dns.sh
	registry_addr="$(E2E_REGISTRY_DNS_NAME)"; \
	registry_ip="$$(. "$(E2E_GENERATED)/registry.env"; printf '%s' "$${E2E_REGISTRY_SELECTED_IP}")"; \
	if [ "$(E2E_REGISTRY_PORT)" != "443" ]; then \
		registry_addr="$${registry_addr}:$(E2E_REGISTRY_PORT)"; \
	fi; \
	curl -fsS --resolve "$(E2E_REGISTRY_DNS_NAME):$(E2E_REGISTRY_PORT):$${registry_ip}" "https://$${registry_addr}/v2/" >/dev/null

.PHONY: e2e-test
e2e-test:
	GOCACHE=$(GOCACHE) go test -tags=e2e -v -timeout=5m -count=1 ./tests/e2e/...

.PHONY: e2e-undeploy
e2e-undeploy:
	$(KUBECTL) delete namespace $(E2E_NAMESPACE) --ignore-not-found

.PHONY: e2e-clean
e2e-clean:
	$(K3D) cluster delete $(E2E_CLUSTER)

.PHONY: e2e
e2e: e2e-cluster e2e-build e2e-deploy e2e-test

.PHONY: e2e-current
e2e-current: e2e-deploy e2e-test

.PHONY: e2e-current-push
e2e-current-push: e2e-image-push e2e-current

.PHONY: e2e-current-registry
e2e-current-registry: e2e-registry-nodeport
	registry_host="$(E2E_REGISTRY_HOST)"; \
	if [ -z "$${registry_host}" ]; then \
		registry_host="$$( $(KUBECTL) get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}' )"; \
	fi; \
	if [ -z "$${registry_host}" ]; then \
		echo "E2E_REGISTRY_HOST is empty; set it to a Kubernetes node IP reachable from this machine and the Talos nodes." >&2; \
		exit 1; \
	fi; \
	$(MAKE) E2E_IMAGE="$${registry_host}:$(E2E_REGISTRY_NODE_PORT)/nebula-mnemosina:e2e" E2E_IMAGE_PULL_POLICY=Always CONTAINER_PUSH_FLAGS=--tls-verify=false e2e-image-push e2e-current

.PHONY: e2e-current-registry-https
e2e-current-registry-https: e2e-registry-https
	registry_addr="$(E2E_REGISTRY_DNS_NAME)"; \
	if [ "$(E2E_REGISTRY_PORT)" != "443" ]; then \
		registry_addr="$${registry_addr}:$(E2E_REGISTRY_PORT)"; \
	fi; \
	$(MAKE) E2E_IMAGE="$${registry_addr}/nebula-mnemosina:e2e" E2E_IMAGE_PULL_POLICY=Always e2e-image-push e2e-current

.PHONY: e2e-redeploy
e2e-redeploy: e2e-build e2e-deploy e2e-test
