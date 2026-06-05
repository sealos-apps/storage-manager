# Sealos Storage Manager Viewer

Encore.go backend and Vite React frontend for creating temporary File Browser
viewer sessions over Kubernetes PVCs.

## Local Files

Do not commit local credentials or generated runtime config:

- `config/kubeconfig.dev.yaml`
- `config/kubeconfig.management.yaml`
- `config/viewer.yaml`
- `config/viewer.debug.yaml`
- `config/viewer.integration.yaml`
- `config/*.local.yaml`

Use the committed templates for local files:

```sh
cp config/viewer.example.yaml config/viewer.yaml
cp config/viewer.debug.example.yaml config/viewer.debug.yaml
cp config/viewer.integration.example.yaml config/viewer.integration.yaml
```

## Quality Gates

```sh
make fmt
make verify
```

Top-level Makefile targets cover backend and frontend where both sides have a
matching check. Use scoped targets when you only need one side:

```sh
make backend-verify
make web-verify
make backend-test
make web-test
```

Optional tools used by the full plan:

```sh
make lint
make security
make build-image IMAGE=registry.example.com/viewer-backend:dev
```

`make test-integration` uses a real Kubernetes cluster and reads
`debug.management_kubeconfig_path` plus optional `debug.forced_namespace` from
`config/viewer.integration.yaml`. Local kubeconfigs and integration config
files are ignored by git.

## Local Dev Server

This repository is configured for self-hosted Encore development without
Encore Cloud. Keep `encore.app` unlinked (`"id": ""`) so Encore CLI commands
use the local app identity and do not fetch development secrets from Encore
Cloud.

```sh
make dev
```

`make dev` starts the backend Encore server and the Vite frontend dev server in
parallel. The backend serves on `0.0.0.0:4000` and reads business configuration
from `config/viewer.yaml`. The frontend dev server receives:

```sh
VITE_API_BASE_URL="http://localhost:4000"
VITE_DEV_KUBECONFIG="$(cat ../config/kubeconfig.dev.yaml)"
```

In Vite dev mode, `VITE_API_BASE_URL` takes precedence over
`/runtime-config.js`, so local development talks directly to the Encore server
without needing the production `/api/*` nginx rewrite.
Production builds reject `VITE_API_BASE_URL`; use `runtime-config.js` for
deploy-time API root overrides.

Override those defaults when needed:

```sh
make web-dev WEB_DEV_API_BASE_URL=http://localhost:4000 WEB_DEV_KUBECONFIG=../config/kubeconfig.dev.yaml
```

Use scoped dev targets when you only need one process:

```sh
make backend-dev
make web-dev
```

Use the Makefile targets for local validation so tests run through Encore's
code generation and runtime setup:

```sh
make test
make backend-test
make web-test
make test-race
make test-integration
```

`make test` runs backend Encore tests and frontend Vitest tests. `make
test-race` wraps backend `encore test -race`; Encore CLI v1.57.4 currently
crashes in its race runtime on macOS arm64, so the default `make verify` gate
uses `make backend-test` instead.

The backend config path is selected by the `CONFIG` environment variable. The
Makefile sets it for local Encore commands:

```sh
make dev CONFIG=config/viewer.debug.yaml
make test-integration INTEGRATION_CONFIG=config/viewer.integration.yaml
```

Frontend commands can be reached through Makefile wrappers:

```sh
make web-install
make web-generate-api
make web-typecheck
make web-build
make web-check-css
make web-e2e
```

Frontend `VITE_*` variables only affect the Vite workspace under `web/`.
Self-hosted runtime variables referenced by `infra-config.json`, such as
`PROMETHEUS_REMOTE_WRITE_URL`, stay separate from viewer business config.
Keep local development kubeconfigs under `config/`: use
`config/kubeconfig.dev.yaml` for frontend user authorization and
`config/kubeconfig.management.yaml` for backend debug/integration management
access.

All public endpoints are typed Encore APIs so OpenAPI/client generation includes
request and response schemas:

```sh
encore gen client --lang=openapi --output openapi.json
```

`GET /metrics` is the only raw endpoint. It is reserved for Prometheus text
scraping and local debugging, so it is exempt from the business endpoint schema
rule.

Application metrics use `encore.dev/metrics` counters. In self-hosted images,
Encore exports them according to `infra-config.json`, for example through the
Prometheus `remote_write_url` environment reference. Operation logs use
`encore.dev/rlog`; Encore attaches those logs to its built-in distributed traces
when `log_config` is set to `trace` in the infra config.

External distributed tracing is optional and limited to traces. It reuses the
existing operation instrumentation and wraps File Browser and Kubernetes HTTP
clients with OpenTelemetry propagation. Keep it disabled for fully local
offline development, or enable the OTLP HTTP exporter in `viewer.yaml`:

```yaml
observability:
  service_name: sealos-storage-manager-viewer
  logs:
    exporter: encore
    level: info
  traces:
    exporter: otlp
    endpoint: http://otel-collector:4318/v1/traces
    sample_ratio: 1
    batch_timeout: 5s
    export_timeout: 5s
```

## Encore MCP

Encore MCP is intentionally not configured for offline development because it
requires a Cloud app id. Do not put kubeconfig, token, or cluster secrets in MCP
configuration.

## Self-hosted Build

```sh
make build-image IMAGE=sealos-storage-manager-viewer:dev
```

The frontend is a separate Vite build. Build `web/`, copy `web/dist` into an
nginx-compatible static image, and use `sealos-storage-manager-web:dev` or your
registry image name in the frontend deployment manifest.

Deploy the backend image with the existing backend manifests in `deploy/`,
mounting a real `viewer.yaml` through a ConfigMap or Secret. Deploy the
frontend with:

```sh
kubectl apply -f deploy/namespace.yaml
kubectl apply -f deploy/configmap.yaml
kubectl apply -f deploy/service-account.yaml
kubectl apply -f deploy/storageclass-admin.yaml
kubectl apply -f deploy/deployment.yaml
kubectl apply -f deploy/service.yaml
kubectl apply -f deploy/web-configmap.yaml
kubectl apply -f deploy/web-deployment.yaml
kubectl apply -f deploy/web-service.yaml
```

Expose `viewer-web` as the public entrypoint. The committed frontend nginx
config serves the SPA, rewrites public `/api/*` requests to the backend's
unprefixed routes, and proxies `/metrics` plus
`/internal/filebrowser-hook/verify` to the internal `viewer-backend` service.

Override frontend runtime settings by replacing the `runtime-config.js` value in
`deploy/web-configmap.yaml` or by mounting your own file at the same path. The
default `apiBaseUrl` is `/api`, which keeps browser API requests on the same
origin as the web app and lets the frontend service own the public rewrite.
