# Sealos Storage Manager Agent Guide

This repository is an Encore.go backend for creating temporary File Browser
viewer sessions over Kubernetes PVCs. Keep this file focused on repository
rules that must shape code changes. Do not turn it into a generic Encore manual.

## Current Backend Shape

- Development and tests run through Encore CLI. Use the Makefile wrappers.
- There is no standalone `cmd/` development server. Do not reintroduce one
  unless the project explicitly stops using Encore CLI for local development.
- Keep `encore.app` unlinked for offline/self-hosted development. Do not require
  Encore Cloud login or Encore MCP for normal local work.
- Business configuration lives in `config/viewer.yaml` locally and is ignored by
  git. Keep committed defaults in `config/viewer.example.yaml` and deployment
  examples in `deploy/configmap.yaml`.
- Local development kubeconfigs live under `config/`: use
  `config/kubeconfig.dev.yaml` for frontend user authorization and
  `config/kubeconfig.management.yaml` for backend debug/integration management
  access. Keep both ignored by git.
- Self-hosted runtime configuration such as metrics/log export belongs in
  `infra-config.json` and deploy-time config, not in business endpoint code.
- Do not commit kubeconfigs, tokens, cluster credentials, generated Encore
  files, `.omx/`, local config, or unrelated worktree artifacts.

## API Rules

- All business endpoints must be typed Encore APIs with explicit request and
  response structs, so OpenAPI/client generation has schemas.
- Avoid raw endpoints. `GET /metrics` is the only allowed raw endpoint because
  it returns Prometheus text and is not a business API.
- When adding or changing endpoints, update tests and verify schema generation
  assumptions. Use Encore path/query/header/body tags intentionally.
- Keep endpoint handlers thin. Put behavior in `internal/` packages or service
  collaborators that are easy to unit test.
- Propagate `context.Context` through every request path. Do not replace a
  request context with `context.Background()` inside handlers or services.

## Observability Rules

Use the existing observability package instead of adding ad hoc logging,
metrics, or tracing.

- Logs are Encore-owned. Use `Recorder.Logger()` / structured `slog.Attr`
  fields so logs flow through `encore.dev/rlog` in Encore runtime.
- Metrics are Encore-owned. Add counters through the existing
  `encore.dev/metrics` bridge and mirrored local `/metrics` output when a new
  operation needs metrics.
- External tracing is optional and trace-only. It is configured by
  `observability.traces` in `viewer.yaml` and exported through OTLP when
  enabled. Do not add OTel metrics or OTel logs without revisiting the
  self-hosted observability contract.
- Use `Recorder.TraceOperation(ctx, name, attrs...)` around meaningful
  operations. It records operation logs, metrics, and optional OTel spans.
- Always finish trace operations with the returned error:

```go
ctx, finish := recorder.TraceOperation(ctx, "viewer.create_session",
	slog.String("namespace", namespace),
)
defer func() {
	finish(err)
}()
```

Add trace boundaries for:

- Kubernetes API interactions and reconciliation/cleanup work.
- File Browser login or other File Browser HTTP calls.
- Auth/session lifecycle operations.
- Large calculations, loops over many Kubernetes resources, or other code where
  latency/debuggability matters.
- External network calls or future storage/database clients.

Keep trace/log attributes bounded and safe:

- Prefer stable IDs, operation names, namespaces, resource kinds, counts, and
  result labels.
- Do not log or trace kubeconfig contents, tokens, passwords, raw auth headers,
  or File Browser bearer tokens.
- Avoid high-cardinality metric labels and unbounded trace attribute values.

## Configuration Rules

When adding configuration:

- Update `internal/config.Config`, `Default()`, `Validate()`, and redaction if
  needed.
- Update `config/viewer.example.yaml`.
- Update `deploy/configmap.yaml` when the setting is needed in deployed
  self-hosted environments.
- Add config tests for parsing, defaults, validation errors, and embedded deploy
  config where relevant.
- Keep local-only secrets in ignored files or deploy-time secret managers.

## Testing And Quality Gates

Use Makefile targets so Encore code generation/runtime setup and frontend checks
are included. Top-level targets cover the full repository when both backend and
frontend have matching commands.

Required before completing full-stack or cross-cutting changes:

```sh
go mod tidy          # when imports/dependencies changed
make verify          # runs backend and frontend verification
encore check
git diff --check
```

Additional gates:

```sh
make lint            # backend golangci-lint plus frontend eslint
make security        # for security-sensitive changes, requires govulncheck
make test-race       # when race risk is relevant and local Encore runtime supports it
make test-integration CONFIG=config/viewer.yaml
```

Scoped targets are available when a change only affects one side:

```sh
make backend-verify
make web-verify
make backend-test
make web-test
make backend-dev
make web-dev
```

`make web-dev` injects `VITE_API_BASE_URL=http://localhost:4000` and reads
`VITE_DEV_KUBECONFIG` from `../config/kubeconfig.dev.yaml` by default.
Override `WEB_DEV_API_BASE_URL` or `WEB_DEV_KUBECONFIG` when local paths differ.

Testing rules:

- Use `encore test`, not raw `go test ./...`, for repository-wide tests.
- Add or update focused unit tests for behavior changes.
- Add regression coverage before cleanup/refactor work when behavior is not
  already protected.
- Keep integration tests behind explicit config/kubeconfig paths; they must not
  require committed credentials.
- If a quality gate cannot run because a local tool is missing, report that
  exact reason. Do not claim it passed.

## Frontend Rules

All frontend code lives under `web/`. Treat the frontend as a separate Vite
workspace inside this Encore repository; do not scatter React code, generated
clients, or frontend build output outside `web/`.

Required frontend stack:

- Package manager: PNPM.
- Build/runtime: Vite latest, React latest, TypeScript 6.
- Styling: Tailwind CSS latest, shadcn/ui-style primitives, `tailwind-merge`,
  `class-variance-authority`, and `clsx`.
- State/data/forms: TanStack Query, TanStack Form, TanStack Store, and
  TanStack Devtools.
- Tests: Vitest, jsdom, React Testing Library, jest-dom, and user-event.
- Backend SDK: Encore-generated TypeScript client only. Generate it with
  `pnpm generate:api` from `web/`; keep generated client output under
  `web/src/services/encore/client.ts`.

Frontend development flow:

```sh
make web-install
make web-dev
make web-lint
make web-test
make web-typecheck
make web-build
make web-check-css
```

Run `make web-build && make web-check-css` before claiming Chrome compatibility
work is complete. Use `make web-test` for unit/integration checks and
`cd web && pnpm test:watch` only for active local iteration.

Frontend SDK rules:

- Do not hand-write backend request paths when the Encore SDK can represent the
  endpoint.
- Keep SDK imports behind service adapters or feature-local `api/` modules, not
  directly in deeply nested UI components.
- Regenerate the SDK after backend API shape changes and commit generated SDK
  files only when they are intended source artifacts for the frontend. Do not
  commit unrelated Encore-generated backend files.
- Keep API query/mutation options testable. Prefer TanStack Query option
  factories and focused tests around cache keys, params, and mapping behavior.

Frontend code style:

- Use TS6-native config. Do not reintroduce deprecated `baseUrl` or
  `ignoreDeprecations` compatibility in `web/tsconfig*.json`.
- Use Antfu ESLint config from `web/eslint.config.js`.
- Use tabs for indentation with `indent_size = 4`, as defined in
  `web/.editorconfig`.
- Keep `strict`, `noUncheckedIndexedAccess`, `noImplicitOverride`,
  `noUnusedLocals`, and `noUnusedParameters` enabled.
- Prefer type-only imports and keep import ordering compliant with Antfu rules.
- Use `@/*` path aliases for source imports.
- Keep shared UI primitives in `src/components/ui`; do not bury reusable shadcn
  primitives inside feature folders.
- Use `lucide-react` icons for tool buttons and visible commands when an icon is
  appropriate.
- Do not add frontend dependencies unless they are part of the requested stack
  or necessary for the feature. Keep dependencies at latest unless the user
  explicitly asks otherwise.

Frontend directory structure:

```text
web/src/
  app/                 app shell and providers
  assets/              static assets imported by React
  components/ui/       shared shadcn/ui-style primitives
  config/              frontend environment and app config
  features/            feature modules
  hooks/               shared React hooks
  layouts/             route/page layout composition
  pages/               page-level composition
  services/            API clients and shared service adapters
  services/encore/     Encore TypeScript SDK boundary
  store/               global frontend stores
  styles/              Tailwind and global styles
  test/                shared test setup and render helpers
  types/               ambient and shared project types
  utils/               shared helpers
```

Feature modules may own local `api/`, `components/`, `forms/`, `stores/`, and
`types/` folders. Keep cross-feature coupling through shared services, shared
UI primitives, or explicit feature APIs.

Frontend testing rules:

- Every new feature must include unit tests for pure logic, API query options,
  stores, validators, and helpers.
- Every user-visible workflow must include an integration test covering the
  React component behavior across forms, stores, query state, and SDK adapters
  where relevant.
- Co-locate tests with feature code using `*.test.ts` or `*.test.tsx`.
- Use `src/test/render.tsx` for provider-aware integration tests.
- Do not rely on jsdom for visual/browser compatibility claims. Use build
  output checks and browser verification when layout or compatibility matters.

Frontend compatibility rules:

- Chrome 86 is the minimum usable browser, not the only browser target.
- Keep latest Tailwind/shadcn/Vite/React/TanStack versions; solve old-browser
  support with build transforms and polyfills, not dependency downgrades.
- Keep `build.target` and `cssTarget` at `chrome86` unless the support policy
  changes.
- Keep PostCSS compatibility passes for Tailwind v4 output, including cascade
  layers, OKLCH/OKLab, `color-mix()`, modern selectors, logical properties,
  media query ranges, nesting, independent transform fallbacks, empty CSS
  variable fallback spacing, variable-based `color-mix()`, and OKLab gradient
  fallbacks.
- Keep runtime polyfills in `src/polyfills.ts`, including `core-js`,
  `container-query-polyfill`, and `css-has-pseudo/browser`.
- Avoid CSS features that cannot be reliably polyfilled for Chrome 86:
  `field-sizing`, `@starting-style`, `transition-behavior: allow-discrete`,
  `text-wrap: balance`, and `scrollbar-gutter`.
- Treat `:where()` fallback as specificity-changing. Verify visual regressions
  when adding complex selectors.

Encore toolbar rules:

- Local Vite development must inject the Encore toolbar before the React bundle
  using `web/vite/encore-toolbar.ts`.
- Production builds must not include the toolbar script.
- Keep unit tests for toolbar injection behavior when changing Vite plugin
  behavior.

## Code Quality Rules

- Keep diffs small, reviewable, and reversible.
- Prefer deleting unused code over preserving compatibility paths. The backend
  is not yet public production API, so avoid speculative compatibility layers.
- Reuse existing packages and interfaces before adding abstractions.
- Do not add dependencies unless they are necessary and justified by the task.
- Return errors with context. Log errors once at the appropriate boundary; avoid
  logging and returning the same failure repeatedly.
- Use structured data/parsers instead of string manipulation when a typed API is
  available.
- Avoid hidden global state except for Encore resource declarations or explicit
  runtime singletons already established by the project.
- Respect user worktree changes. Do not revert unrelated dirty files.

## Git And Commit Rules

- Stage only files related to the task. Do not stage `.omx/`, local config,
  kubeconfigs, generated Encore files, unrelated UI artifacts, or incidental
  deploy metadata changes.
- Commit messages should explain why the change was made, not just what changed.
- Use git trailers when they add useful future context:

```text
Intent line explaining why

Short context and approach rationale.

Constraint: External constraint that shaped the decision
Rejected: Alternative considered | reason
Confidence: low|medium|high
Scope-risk: narrow|moderate|broad
Directive: Forward-looking warning for future modifiers
Tested: Commands or checks that passed
Not-tested: Known verification gaps
```

## Quick Reference

Common commands:

```sh
make dev
make test
make verify
make backend-verify
make web-verify
encore check
make build-image IMAGE=sealos-storage-manager-viewer:dev
```

Common files:

- `viewer/api.go`: Encore API surface.
- `viewer/runtime.go`: runtime wiring, Encore metrics bridge, Kubernetes and
  File Browser client wiring.
- `internal/observability/`: logging, metrics, and optional OTel trace export.
- `internal/config/`: YAML config model, defaults, validation, deploy config
  tests.
- `internal/kube/`: Kubernetes client abstraction and observed wrapper.
- `internal/filebrowser/`: File Browser HTTP client and auth helpers.
