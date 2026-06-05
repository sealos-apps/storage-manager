GO ?= go
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
ENCORE ?= encore
PNPM ?= pnpm
CONFIG ?= config/viewer.debug.yaml
INTEGRATION_CONFIG ?= config/viewer.integration.yaml
IMAGE ?= sealos-storage-manager-viewer:dev
WEB_DIR ?= web
WEB_DEV_API_BASE_URL ?= http://localhost:4000
WEB_DEV_KUBECONFIG ?= ../config/kubeconfig.dev.yaml

.PHONY: dev backend-dev web-dev fmt backend-fmt web-fmt fmt-check backend-fmt-check web-fmt-check lint backend-lint web-lint vet backend-vet test backend-test web-test test-race backend-test-race test-integration backend-test-integration security backend-security build-image backend-build-image verify backend-verify web-verify tidy backend-tidy web-install web-generate-api web-typecheck build web-build web-check-css e2e web-e2e

dev:
	$(MAKE) -j2 backend-dev web-dev

backend-dev:
	CONFIG=$(CONFIG) $(ENCORE) run --listen 0.0.0.0:4000 --browser=never

web-dev:
	cd $(WEB_DIR) && VITE_API_BASE_URL="$(WEB_DEV_API_BASE_URL)" VITE_DEV_KUBECONFIG="$$(cat $(WEB_DEV_KUBECONFIG))" $(PNPM) dev

fmt: backend-fmt web-fmt

backend-fmt:
	$(GO) fmt ./...

web-fmt:
	cd $(WEB_DIR) && $(PNPM) exec eslint . --fix

fmt-check: backend-fmt-check web-fmt-check

backend-fmt-check:
	@test -z "$$($(GO) fmt ./...)" || (echo "gofmt changed files; run make fmt" && exit 1)

web-fmt-check:
	cd $(WEB_DIR) && $(PNPM) lint

lint: backend-lint web-lint

backend-lint:
	$(GOLANGCI_LINT) run ./...

web-lint:
	cd $(WEB_DIR) && $(PNPM) lint

vet: backend-vet

backend-vet:
	$(GO) vet ./...

test: backend-test web-test

backend-test:
	$(ENCORE) test ./...

web-test:
	cd $(WEB_DIR) && $(PNPM) test

test-race: backend-test-race

backend-test-race:
	$(ENCORE) test -race ./...

test-integration: backend-test-integration

backend-test-integration:
	CONFIG=$(INTEGRATION_CONFIG) $(ENCORE) test -tags=integration ./test/integration -count=1

security: backend-security

backend-security:
	$(GOVULNCHECK) ./...

build-image: backend-build-image

backend-build-image:
	$(ENCORE) build docker --config=infra-config.json $(IMAGE)

tidy: backend-tidy

backend-tidy:
	$(GO) mod tidy

web-install:
	cd $(WEB_DIR) && $(PNPM) install

web-generate-api:
	cd $(WEB_DIR) && $(PNPM) generate:api

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
