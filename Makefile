GO ?= go
GO_VERSION_FILE ?= .go-version
ENCORE_VERSION_FILE ?= .encore-version
GO_BIN_DIR := $(shell dirname "$$(command -v $(GO) 2>/dev/null || echo $(GO))")
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
GOVULNCHECK_VERSION ?= v1.3.0
ENCORE ?= encore
ENCORE_GO_ROOT ?= $(shell $(ENCORE) daemon env 2>/dev/null | sed -n 's/^ENCORE_GOROOT=//p')
ENCORE_GO ?= $(ENCORE_GO_ROOT)/bin/go
ENCORE_ENV := ENCORE_GOROOT="$(ENCORE_GO_ROOT)" PATH="$(GO_BIN_DIR):$(PATH)"
PNPM ?= pnpm
HELM ?= helm
CONFIG ?= config/viewer.debug.yaml
INTEGRATION_CONFIG ?= config/viewer.integration.yaml
IMAGE ?= sealos-storage-manager-viewer:dev
REGISTRY ?=
IMAGE_PREFIX ?= sealos-storage-manager
TAG ?= dev
TAGS ?= $(TAG)
PLATFORMS ?= linux/amd64
WEB_DIR ?= web
WEB_DEV_API_BASE_URL ?= http://localhost:4000
WEB_DEV_KUBECONFIG ?= ../config/kubeconfig.dev.yaml

.PHONY: check-go-version dev backend-dev web-dev fmt backend-fmt web-fmt fmt-check backend-fmt-check web-fmt-check lint backend-lint web-lint vet backend-vet test backend-test web-test test-race backend-test-race test-integration backend-test-integration security backend-security build-image backend-build-image build-images push-images web-build-image chart-lint chart-template chart-package deploy-verify verify backend-verify web-verify tidy backend-tidy web-install web-generate-api web-typecheck build web-build web-check-css e2e web-e2e

check-go-version:
	@required="$$(cat $(GO_VERSION_FILE))"; \
	encore_required="$$(cat $(ENCORE_VERSION_FILE))"; \
	encore_version="$$( { $(ENCORE) version 2>/dev/null || true; } | sed -n 's/^encore version v//p')"; \
	if [ "$$encore_version" != "$$encore_required" ]; then \
		echo "Encore $$encore_required required by $(ENCORE_VERSION_FILE); found $$encore_version. Install the pinned Encore CLI version."; \
		exit 1; \
	fi; \
	if ! command -v $(GO) >/dev/null 2>&1; then \
		echo "Go $$required required by $(GO_VERSION_FILE); $(GO) is not available on PATH."; \
		exit 1; \
	fi; \
	actual="$$(GOTOOLCHAIN=local $(GO) env GOVERSION | sed -e 's/^go//' -e 's/-encore$$//')"; \
	if [ "$$actual" != "$$required" ]; then \
		echo "Go $$required required by $(GO_VERSION_FILE); found $$actual. Install Go $$required or point GO at that binary."; \
		exit 1; \
	fi; \
	if [ ! -x "$(ENCORE_GO)" ]; then \
		echo "Encore Go runtime not found at $(ENCORE_GO). Reinstall Encore $$encore_required."; \
		exit 1; \
	fi; \
	encore_actual="$$(GOTOOLCHAIN=local $(ENCORE_GO) env GOVERSION 2>/dev/null | sed -e 's/^go//' -e 's/-encore$$//')"; \
	if [ "$$encore_actual" != "$$required" ]; then \
		echo "Encore Go $$required required by $(GO_VERSION_FILE); found $$encore_actual at $(ENCORE_GO). Install an Encore CLI built with Go $$required."; \
		exit 1; \
	fi

dev: check-go-version
	$(MAKE) -j2 backend-dev web-dev

backend-dev: check-go-version
	$(ENCORE_ENV) CONFIG=$(CONFIG) $(ENCORE) run --listen 0.0.0.0:4000 --browser=never

web-dev:
	cd $(WEB_DIR) && $(ENCORE_ENV) VITE_API_BASE_URL="$(WEB_DEV_API_BASE_URL)" VITE_DEV_KUBECONFIG="$$(cat $(WEB_DEV_KUBECONFIG))" $(PNPM) dev

fmt: backend-fmt web-fmt

backend-fmt: check-go-version
	GOTOOLCHAIN=local $(GO) fmt ./...

web-fmt:
	cd $(WEB_DIR) && $(PNPM) exec eslint . --fix

fmt-check: backend-fmt-check web-fmt-check

backend-fmt-check: check-go-version
	@out="$$(GOTOOLCHAIN=local $(GO) fmt ./... 2>&1)"; status=$$?; \
	if [ $$status -ne 0 ]; then \
		echo "$$out"; \
		exit $$status; \
	fi; \
	if [ -n "$$out" ]; then \
		echo "$$out"; \
		echo "gofmt changed files; run make fmt"; \
		exit 1; \
	fi

web-fmt-check:
	cd $(WEB_DIR) && $(PNPM) lint

lint: backend-lint web-lint

backend-lint: check-go-version
	$(GOLANGCI_LINT) run ./...

web-lint:
	cd $(WEB_DIR) && $(PNPM) lint

vet: backend-vet

backend-vet: check-go-version
	$(GO) vet ./...

test: backend-test web-test

backend-test: check-go-version
	$(ENCORE_ENV) $(ENCORE) test ./...

web-test:
	cd $(WEB_DIR) && $(PNPM) test

test-race: backend-test-race

backend-test-race: check-go-version
	$(ENCORE_ENV) $(ENCORE) test -race ./...

test-integration: backend-test-integration

backend-test-integration: check-go-version
	$(ENCORE_ENV) CONFIG=$(INTEGRATION_CONFIG) $(ENCORE) test -tags=integration ./test/integration -count=1

security: backend-security

backend-security: check-go-version
	@if command -v $(GOVULNCHECK) >/dev/null 2>&1; then \
		$(GOVULNCHECK) ./...; \
	else \
		$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...; \
	fi

build-image: backend-build-image

backend-build-image: check-go-version
	$(ENCORE_ENV) $(ENCORE) build docker --config=infra-config.json $(IMAGE)

build-images:
	REGISTRY="$(REGISTRY)" IMAGE_PREFIX="$(IMAGE_PREFIX)" TAGS="$(TAGS)" PLATFORMS="$(PLATFORMS)" PUSH=false ./scripts/build-images.sh

push-images:
	REGISTRY="$(REGISTRY)" IMAGE_PREFIX="$(IMAGE_PREFIX)" TAGS="$(TAGS)" PLATFORMS="$(PLATFORMS)" PUSH=true ./scripts/build-images.sh

web-build-image:
	docker buildx build --platform $(PLATFORMS) -f $(WEB_DIR)/Dockerfile -t $(IMAGE_PREFIX)-web:$(TAG) --load $(WEB_DIR)

chart-lint:
	$(HELM) lint deploy

chart-template:
	$(HELM) template sealos-storage-manager deploy --namespace sealos-storage-manager >/dev/null

chart-package:
	mkdir -p dist/charts
	$(HELM) package deploy --destination dist/charts

deploy-verify: chart-lint chart-template chart-package

tidy: backend-tidy

backend-tidy: check-go-version
	$(GO) mod tidy

web-install:
	cd $(WEB_DIR) && $(ENCORE_ENV) $(PNPM) install

web-generate-api:
	cd $(WEB_DIR) && $(ENCORE_ENV) $(PNPM) generate:api

web-typecheck:
	cd $(WEB_DIR) && $(PNPM) exec tsc -b

build: web-build

web-build:
	cd $(WEB_DIR) && $(PNPM) build

web-check-css:
	cd $(WEB_DIR) && $(PNPM) check:css

e2e: web-e2e

web-e2e:
	cd $(WEB_DIR) && $(PNPM) e2e

web-verify: web-lint web-test web-typecheck web-build web-check-css

backend-verify: backend-fmt-check backend-vet backend-test

verify: backend-verify web-verify
