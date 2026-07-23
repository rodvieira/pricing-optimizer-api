# Pricing Optimizer API

[![CI](https://github.com/rodvieira/pricing-optimizer-api/actions/workflows/ci.yml/badge.svg)](https://github.com/rodvieira/pricing-optimizer-api/actions/workflows/ci.yml)
[![Deploy](https://github.com/rodvieira/pricing-optimizer-api/actions/workflows/deploy.yml/badge.svg)](https://github.com/rodvieira/pricing-optimizer-api/actions/workflows/deploy.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/rodvieira/pricing-optimizer-api)](https://goreportcard.com/report/github.com/rodvieira/pricing-optimizer-api)
[![Coverage](https://img.shields.io/badge/coverage-80%25%2B%20floor%20(usecase%2Fdomain)-brightgreen)](./.github/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

Go backend for **Pricing Optimizer**: paste a product URL, get three AI-generated
pricing-page variations — each built around a distinct psychological pricing strategy —
streamed live over Server-Sent Events.

**Live**: `https://pricing-optimizer-api-hnzq7nvuqq-uc.a.run.app` · Frontend:
[`pricing-optimizer-web`](https://github.com/rodvieira/pricing-optimizer-web)
([live app](https://pricing-optimizer-web.vercel.app))

```bash
curl https://pricing-optimizer-api-hnzq7nvuqq-uc.a.run.app/v1/healthz
# {"status":"ok","version":"..."}
```

A real `POST /v1/analyze` round-trip against the live deploy, not a fixture:

```bash
curl -X POST https://pricing-optimizer-api-hnzq7nvuqq-uc.a.run.app/v1/analyze \
  -H "Content-Type: application/json" \
  -d '{"url":"https://linear.app"}'
```

```json
{
  "url": "https://linear.app",
  "title": "Linear – The system for product development",
  "valueProposition": "Linear is a product development system that helps teams plan, build, and launch products with AI workflows at its core.",
  "industry": "developer-tools",
  "audience": {
    "segment": "product development teams",
    "sophistication": "high",
    "pricePosition": "premium"
  },
  "keywords": ["product development", "AI workflows", "team collaboration"],
  "sourceType": "static",
  "analyzedAt": "2026-07-23T23:02:08.287572969Z"
}
```

## What it does

1. **Analyze** — scrapes a product URL (a fast static fetch first, a real headless
   browser if the page turns out to be a client-rendered SPA), then an LLM classifies
   the target audience and extracts the value proposition.
2. **Generate** — three pricing strategies (anchor, freemium ladder, value-based) are
   generated in parallel via structured LLM tool calling, streamed to the client as they
   complete rather than after a single blocking call.
3. **Export** — any completed variation renders to a JSX component, standalone HTML, or
   a Stripe Pricing Table JSON config.

No route ships ahead of its contract: every endpoint is defined in `openapi.yaml` first,
then generated into Go handlers/types via `oapi-codegen` — the same spec generates the
frontend's TypeScript client.

## Architecture

Clean Architecture, four layers, dependencies point one direction only:

```
cmd/api/            entry point — wiring, graceful shutdown, nothing else
internal/
  domain/            entities + ports (interfaces). Imports nothing.
  usecase/            orchestration. Imports domain only — never an adapter directly.
  adapter/
    httpapi/           Chi router, handlers, SSE writer
    scraper/            chromedp (SPA) + colly (static), composed behind one interface
    llm/                Anthropic + Groq behind one LLMProvider interface
    repository/         pgx + sqlc
    cache/              Redis (rate limiting, idempotency, response cache)
  config/             env-based configuration (12-factor)
  telemetry/          OpenTelemetry tracing + Sentry error tracking, both no-op by default
db/
  migrations/          goose
  queries/             sqlc
```

`usecase` never imports an `adapter` package directly — every external dependency
(scraper, LLM provider, repository, cache) is a `domain`-defined interface, satisfied by
an `adapter` implementation and wired together only in `cmd`. Swapping Anthropic for Groq,
or Postgres for something else, never touches business logic.

## Key engineering decisions

A few decisions worth calling out — the full log (14 ADRs) lives alongside the code
history; these are the ones that shaped the system most:

- **Contract-first, one spec for two repos.** `openapi.yaml` is hand-authored once and
  drives codegen on both sides (`oapi-codegen` here, `openapi-typescript` in the
  frontend). No endpoint exists in code before it exists in the spec.
- **LLM provider is swappable by env var, not by redeploying different code.** One
  `LLMProvider` interface, two adapters. Anthropic (Claude) in development for the best
  reasoning quality while iterating; Groq in production for near-zero marginal cost at
  comparable quality for this task — with an 8B fallback model if the primary errors.
- **Two-tier scraping, not one.** A fast static fetch (`colly`) is tried first; only
  pages that come back empty (client-rendered SPAs) fall through to a real headless
  Chromium (`chromedp`). Most marketing sites are static — paying the ~10x cost of a
  full browser launch for those would be wasted latency and memory on every request.
- **Idempotency without requiring the client to send a key.** `POST /v1/generate`
  accepts an `Idempotency-Key` header, but derives one implicitly from the request body's
  content hash when the caller doesn't send one — a retried request never double-spends
  an LLM call by accident, with no client-side cooperation required.
- **Synchronous, not batched, telemetry export.** Both OpenTelemetry spans and Sentry
  error events export synchronously per-request rather than on a background timer. Cloud
  Run only allocates CPU while a request is in flight; a background flush goroutine can
  simply never run before the instance freezes, silently dropping every span — found by
  shipping the "recommended" batched default first, then noticing traces never actually
  arrived, then fixing it and confirming delivery for real.
- **$0/month infrastructure by construction, not by accident.** Every managed service
  (Neon Postgres, Upstash Redis, Grafana Cloud, Sentry, Cloud Run itself) was chosen
  specifically for a free tier with no credit-card-triggered surprise, verified against
  current pricing rather than assumed, with `min-instances=0` non-negotiable on Cloud Run.

## API

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/analyze` | Scrape + classify a product URL into a `SiteProfile` |
| `POST` | `/v1/generate` | Stream 3 pricing variations for a `SiteProfile` (SSE) |
| `GET` | `/v1/generations/{id}` | Fetch a previously completed generation |
| `POST` | `/v1/export/{id}` | Export one variation as JSX / HTML / Stripe config |
| `GET` | `/v1/healthz` | Liveness/readiness probe |

Full request/response schemas: [`openapi.yaml`](./openapi.yaml). Try it against the live
deployment:

```bash
curl -X POST https://pricing-optimizer-api-hnzq7nvuqq-uc.a.run.app/v1/analyze \
  -H "Content-Type: application/json" \
  -d '{"url":"https://linear.app"}'
```

## Stack

Go 1.26 · [chi](https://github.com/go-chi/chi) · [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)
· [chromedp](https://github.com/chromedp/chromedp) + [colly](https://github.com/gocolly/colly)
· Anthropic + Groq (OpenAI-compatible client) · [pgx](https://github.com/jackc/pgx) +
[sqlc](https://sqlc.dev/) · [goose](https://github.com/pressly/goose) migrations ·
[go-redis](https://github.com/redis/go-redis) · OpenTelemetry · [Sentry](https://sentry.io) ·
[testify](https://github.com/stretchr/testify) + [testcontainers-go](https://golang.testcontainers.org/)
+ [uber-go/mock](https://github.com/uber-go/mock) · golangci-lint

**Infrastructure**: Google Cloud Run (backend host) · Neon (serverless Postgres) ·
Upstash (serverless Redis) · Grafana Cloud (tracing) · Sentry (error tracking) · GitHub
Actions (CI/CD) — all on free tiers, $0/month by design.

## Development

```bash
cp .env.example .env          # fill in API keys; Postgres/Redis default to local Docker
docker run -d -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:16
docker run -d -p 6379:6379 redis:7
make migrate-up
go run ./cmd/api
```

```bash
make test          # unit tests
make test-race     # + race detector (needs a C toolchain)
make lint          # golangci-lint
```

`make help` lists every task, including OpenAPI codegen regeneration and migration
management.

## Testing

Table-driven tests throughout, `testify` assertions, mocks generated by `uber-go/mock`
(never hand-written). Integration tests spin up a real Postgres via `testcontainers-go`
rather than mocking the database — gated behind `testing.Short()` so `go test -short`
(CI's default) skips them and a local `go test ./...` with Docker running exercises them
for real. Coverage floor: **80%** on `internal/usecase` and `internal/domain`, enforced
in CI.

## Deployment

Push to `main` → GitHub Actions applies pending Postgres migrations, builds the image,
pushes to Artifact Registry, deploys to Cloud Run, then ensures the service stays
publicly invokable (Cloud Run defaults to IAM-only, and this API is called directly from
the browser). No Secret Manager — a single-maintainer $0/month project doesn't need that
extra moving part; secrets live in a GitHub Environment instead, injected straight into
Cloud Run's env vars at deploy time.

## License

MIT — see [`LICENSE`](./LICENSE).
