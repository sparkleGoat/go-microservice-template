# go-microservice-template

A production-grade "golden path" template for Go HTTP microservices — the
reference implementation a platform team hands to product engineers so every
service ships with the same observability, health, security, and deployment
story out of the box.

[![ci](https://github.com/sparkleGoat/go-microservice-template/actions/workflows/ci.yml/badge.svg)](https://github.com/sparkleGoat/go-microservice-template/actions/workflows/ci.yml)
![go](https://img.shields.io/badge/go-1.25-00ADD8)
![license](https://img.shields.io/badge/license-MIT-green)

## Why this exists

On a platform team, the slowest part of standing up a new service is not the
business logic — it is re-deriving the same operational scaffolding every time:
structured logging, metrics, tracing, health probes, graceful shutdown, a
hardened container, and a Helm chart that a cluster will actually accept. This
template encodes those decisions once so a new service is production-ready on
day one and consistent with every other service in the fleet.

## What you get

| Concern | Implementation |
| --- | --- |
| **Routing** | [chi](https://github.com/go-chi/chi) with an explicit, documented middleware order |
| **Structured logging** | `log/slog` JSON handler, service/env fields on every line |
| **Metrics** | Prometheus RED metrics (rate, errors, duration) on a separate `:9090` listener |
| **Tracing** | OpenTelemetry OTLP exporter, parent-based 10% sampling, no-op when no collector |
| **Health** | Kubernetes-correct `/healthz` (liveness) and `/readyz` (readiness w/ dependency probes) |
| **Lifecycle** | Graceful shutdown that flips readiness first, then drains in-flight requests |
| **Config** | Twelve-factor env config with safe defaults, no config files required |
| **Container** | Multi-stage build to a distroless, non-root, read-only-rootfs image (~2MB base) |
| **Deploy** | Helm chart with HPA, ServiceMonitor, topology spread, and a hardened pod spec |
| **CI** | GitHub Actions: gofmt, vet, race tests + coverage, golangci-lint, image build, Trivy scan |

## Architecture

```
                   :8080 (app)                 :9090 (metrics)
                      │                              │
client ──▶ chi router ──┤                              └─▶ /metrics (Prometheus)
                      │
middleware (in order): Recoverer ▸ RequestID ▸ RealIP ▸ logger ▸ metrics
                      │
        ┌─────────────┼─────────────────────────────┐
        ▼             ▼                             ▼
    /healthz       /readyz                    /api/v1/*
   (liveness)   (readiness +                (business logic,
                 dep probes)                 traced spans)
```

The metrics endpoint binds to a **separate port** so it is never exposed through
the public ingress — a common production mistake this template avoids.

## Quick start

```bash
# Run locally (no collector needed; tracing no-ops)
make run

# In another shell:
curl localhost:8080/api/v1/hello?name=mary
curl localhost:8080/readyz
curl localhost:9090/metrics

# Test, lint, build the container
make test
make lint
make docker
```

## Configuration

All configuration is environment-driven (twelve-factor). Every value has a safe
default, so the binary runs unchanged across local, CI, and Kubernetes.

| Variable | Default | Description |
| --- | --- | --- |
| `SERVICE_NAME` | `go-microservice-template` | Name in logs, metrics, traces |
| `ENVIRONMENT` | `dev` | Deployment label |
| `HTTP_ADDR` | `:8080` | Application listen address |
| `METRICS_ADDR` | `:9090` | Prometheus listen address |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `OTLP_ENDPOINT` | _(empty)_ | OTLP collector `host:port`; empty disables tracing |
| `READ_TIMEOUT` | `5s` | Request read timeout (Go duration or bare seconds) |
| `WRITE_TIMEOUT` | `10s` | Response write timeout |
| `SHUTDOWN_TIMEOUT` | `15s` | Graceful drain budget |

## Deploying to Kubernetes

```bash
helm upgrade --install my-service deploy/helm/service \
--set image.tag=1.0.0 \
--set otlpEndpoint=otel-collector.observability:4317
```

The chart ships with:

- **HorizontalPodAutoscaler** (CPU-based, 2–10 replicas by default)
- **ServiceMonitor** for the Prometheus Operator
- **Hardened pod**: non-root uid 65532, read-only rootfs, all capabilities dropped, `RuntimeDefault` seccomp
- **Liveness/readiness probes** wired to `/healthz` and `/readyz`
- **Topology spread** across nodes for availability

## Project layout

```
cmd/server/            entrypoint: config, observability, lifecycle, graceful shutdown
internal/config/       twelve-factor env configuration + tests
internal/observability/logging (slog), metrics (Prometheus RED), tracing (OTel)
internal/health/       liveness/readiness with dependency probes + tests
internal/httpserver/   chi router, middleware, handlers, response envelopes + tests
deploy/helm/service/   production Helm chart (HPA, ServiceMonitor, hardened pod)
.github/workflows/     CI: format, vet, test, lint, image build, Trivy scan
Dockerfile             multi-stage distroless build
```

## Design notes

- **Readiness flips before drain.** On `SIGTERM` the service marks itself
not-ready *before* shutting down the HTTP server, so the load balancer stops
routing new requests while existing ones finish. This is the difference
between zero-downtime and dropped requests during a rolling deploy.
- **Bounded metric cardinality.** The request metrics use the matched chi route
pattern (`/api/v1/hello`) rather than the raw path, so per-id URLs do not
explode Prometheus series counts.
- **Fresh registry per instance.** Metrics are attached to an injected
`prometheus.Registerer`, not package globals, so tests spin up isolated
servers without duplicate-registration panics.
- **Tracing degrades gracefully.** With no `OTLP_ENDPOINT` set, spans become
no-ops and the binary runs anywhere without a collector.

## License

MIT — see [LICENSE](LICENSE).
