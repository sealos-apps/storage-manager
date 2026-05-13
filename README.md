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

The Encore CLI may try to fetch development secrets from Encore Cloud during
`encore run`. For self-hosted development without Encore Cloud, use the local
dev server instead:

```sh
make dev
```

It serves the same REST handlers on `0.0.0.0:4000`, reads all business
configuration from `config/viewer.yaml`, and runs scheduled cleanup from the
configured purge interval.

## Encore MCP

The Encore MCP server should point at the same app id as `encore.app`.

```sh
encore mcp run --app=sealos-storage-manager-viewer
```

For a long-running local MCP server:

```sh
encore mcp start --app=sealos-storage-manager-viewer
```

No kubeconfig, token, or cluster secret belongs in MCP configuration.

## Self-hosted Build

```sh
encore build docker --config=infra-config.json sealos-storage-manager-viewer:dev
```

Deploy the image with the manifests in `deploy/`, mounting a real `viewer.yaml` through a ConfigMap or Secret.
