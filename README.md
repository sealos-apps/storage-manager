# Sealos Storage Manager Viewer Backend

Encore.go backend for creating temporary File Browser viewer sessions over Kubernetes PVCs.

## Local Files

Do not commit local credentials or generated runtime config:

- `kubeconfig.test.yaml`
- `config/viewer.yaml`
- `config/*.local.yaml`

Use `config/viewer.example.yaml` as the committed template.

## Quality Gates

```sh
make fmt
make verify
```

Optional tools used by the full plan:

```sh
make lint
make security
make build-image IMAGE=registry.example.com/viewer-backend:dev
```

`make test-integration` reads the kubeconfig path from the YAML config and is intended for local protected development only.

Create a local `config/viewer.yaml` from `config/viewer.example.yaml` and point
`integration.kubeconfig_path` at `kubeconfig.test.yaml`. Both files are ignored
by git.

## Local Dev Server

This repository is configured for self-hosted Encore development without
Encore Cloud. Keep `encore.app` unlinked (`"id": ""`) so Encore CLI commands
use the local app identity and do not fetch development secrets from Encore
Cloud.

```sh
make dev
```

`make dev` wraps `encore run`, serves on `0.0.0.0:4000`, and reads business
configuration from `config/viewer.yaml`.

Use the Makefile targets for local validation so tests run through Encore's
code generation and runtime setup:

```sh
make test
make test-race
make test-integration CONFIG=config/viewer.yaml
```

`make test-race` also wraps `encore test -race`; Encore CLI v1.57.4 currently
crashes in its race runtime on macOS arm64, so the default `make verify` gate
uses `make test` instead.

All public endpoints are typed Encore APIs so OpenAPI/client generation includes
request and response schemas:

```sh
encore gen client --lang=openapi --output openapi.json
```

`GET /metrics` returns a JSON metrics snapshot rather than Prometheus text so it
can stay documented as a typed endpoint.

## Encore MCP

Encore MCP is intentionally not configured for offline development because it
requires a Cloud app id. Do not put kubeconfig, token, or cluster secrets in MCP
configuration.

## Self-hosted Build

```sh
make build-image IMAGE=sealos-storage-manager-viewer:dev
```

Deploy the image with the manifests in `deploy/`, mounting a real `viewer.yaml` through a ConfigMap or Secret.
