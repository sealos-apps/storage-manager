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
make build-images TAG=dev
make deploy-verify
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
  service_name: storage-manager-viewer
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

Build both deployable images:

```sh
make build-images TAG=dev
```

This creates:

- `storage-manager-backend:dev`
- `storage-manager-web:dev`

Push both images to a registry:

```sh
make push-images REGISTRY=ghcr.io IMAGE_PREFIX=owner/storage-manager TAG=dev
```

The legacy backend-only image target remains available when needed:

```sh
make build-image IMAGE=registry.example.com/viewer-backend:dev
```

## Helm Deployment

`deploy/charts/storage-manager/` is the Helm chart for self-hosted
deployment. Validate it before shipping changes:

```sh
make deploy-verify
```

In Sealos-managed installs, `deploy/entrypoint.sh` injects global HTTP/TLS
settings into Helm from `/root/.sealos/cloud/values/global.yaml` and
`sealos-system/sealos-config`. The chart defaults intentionally omit those
global values.

The chart derives:

- File Browser auth callback URL:
  `https://<web host>/internal/filebrowser-hook/verify`
- File Browser viewer host template:
  `<hostPrefix>-{{ .PodSessionID }}.<cloudDomain>`

Override `user.hookClientToken` before exposing the service. The committed
value is a placeholder token.

Expose `viewer-web` as the public entrypoint. The chart renders nginx config
that serves the SPA, rewrites public `/api/*` requests to the backend's
unprefixed routes, and proxies `/metrics` plus
`/internal/filebrowser-hook/verify` to the internal `viewer-backend` service.

Use `charts/storage-manager/storage-manager-values.yaml` as the user-level
override entrypoint for Sealos installs. It exposes product-facing `user.*`
values such as `user.adminUserIds`, `user.hookClientToken`,
`user.integrations.*`, `user.viewer.*`, `user.web.*`, `user.desktop.enabled`,
and `user.features.*`. The chart's internal `backend.*`, `web.*`, `rbac.*`,
and `desktopApp.*` paths remain available for advanced Helm overrides, but they
are not the recommended install-package interface.

The default `user.web.apiBaseUrl` is `/api`, which keeps browser API requests on
the same origin as the web app and lets the frontend service own the public
rewrite.

## Sealos Cluster Image

`deploy/` is also the Sealos cluster image build context. It contains:

- `Kubefile`, which packages cached runtime images, charts, and the install
  entrypoint.
- `entrypoint.sh`, which sources `/root/.sealos/cloud/scripts/tools.sh`, reads
  global HTTP/TLS settings, loads packaged values plus all
  `/root/.sealos/cloud/values/apps/storage-manager/*-values.yaml`
  overrides, and runs `helm upgrade -i ... --create-namespace`. The chart does
  not render a `Namespace` resource.
- `charts/storage-manager/storage-manager-values.yaml`, the
  user-level packaged values used by Sealos app installs.

The `images` workflow publishes:

- `ghcr.io/<owner>/<repo>/<repo>:<tag>` for the backend runtime
- `ghcr.io/<owner>/<repo>/<repo>-web:<tag>` for the web runtime
- `ghcr.io/<owner>/<repo>/<repo>-cluster:<tag>` for the Sealos cluster image

For each cluster image build, the workflow caches runtime images with
`sealos registry save --registry-dir=registry_<arch> --arch <arch> .`, saves
`storage-manager-cluster-<tag>-<arch>.tar.gz`, generates an md5 file,
and uploads both artifacts to OSS.
