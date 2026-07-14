---
name: "go-best-practices"
description: "Idiomatic Go conventions and this project's Clean Architecture rules. Use before writing or reviewing any Go code in pricing-optimizer-api: package/layer placement, error handling, context, concurrency (errgroup for the 3 parallel generations), interfaces/ports, slog, constructors, and golangci-lint compliance."
argument-hint: "Optional: a file, package, or concern to focus on"
metadata:
  author: "rodvieira"
user-invocable: true
disable-model-invocation: false
---

# Go best practices — pricing-optimizer-api

Apply these when writing or reviewing Go here. They encode both idiomatic Go and this
project's constitution (`.specify/memory/constitution.md`). When a rule below conflicts
with the constitution, the constitution wins.

## 1. Architecture placement (non-negotiable)

Four layers, strict import direction:

- `internal/domain` — entities + ports (interfaces). Imports NOTHING outside stdlib.
- `internal/usecase` — orchestration. Imports `domain` ONLY. No adapters, no HTTP, no SDKs.
- `internal/adapter/*` — implementations of domain ports (httpapi, scraper, llm, repository,
  cache). Imports `domain`. NEVER imports `usecase`.
- `cmd/api` — wires everything at startup.

Before adding code, ask "which layer owns this?" A DB query belongs in `adapter/repository`,
not `usecase`. An LLM prompt belongs in `adapter/llm`, not `domain`. If a use case needs a
capability, define a port for it in `domain` and inject the implementation from `cmd`.

## 2. Interfaces and ports

- Define interfaces where they are CONSUMED (in `domain`, as ports), not where implemented.
- Keep interfaces small (1-3 methods). `LLMProvider`, `Scraper`, `GenerationRepo`, `Cache`.
- Accept interfaces, return concrete structs from constructors.
- No premature interfaces: only introduce a port when a use case depends on it.

## 3. Constructors and dependency injection

- Every adapter/use case exposes `NewX(deps...) *X` (or the interface it satisfies).
- Dependencies are passed in, never constructed inside. `cmd/api/main.go` is the only
  place that knows concrete types and wires the graph.
- No global mutable state. No `init()` for wiring. Config is read once in `cmd`.

## 4. Error handling

- Return errors; do not panic in library code. `panic` is acceptable only for truly
  unreachable programmer errors (e.g. the LLM factory's unknown-provider default).
- Wrap with context using `%w`: `fmt.Errorf("analyze site %q: %w", url, err)`.
- Define sentinel errors in `domain` for conditions use cases branch on
  (`var ErrGenerationNotFound = errors.New("generation not found")`); compare with
  `errors.Is`. Use `errors.As` for typed errors.
- Map domain errors to HTTP Problem responses in `adapter/httpapi`, never leak internal
  error strings to clients.
- Do not log-and-return the same error; handle it at exactly one level.

## 5. Context

- `ctx context.Context` is the FIRST parameter of any function doing I/O, LLM, or DB work.
- Propagate the request context through every layer; never `context.Background()` except in
  `main` and tests.
- Never store a context in a struct.
- Respect cancellation: pass `ctx` to pgx, HTTP clients, and the LLM SDKs so a dropped SSE
  client cancels in-flight generation.

## 6. Concurrency

- The three variations generate in parallel. Use `golang.org/x/sync/errgroup` with a
  context: `g, ctx := errgroup.WithContext(ctx)`; one `g.Go` per strategy; `g.Wait()`
  aggregates the first error and cancels siblings.
- Never leak goroutines: every goroutine has a clear exit tied to `ctx` or a closed channel.
- Protect shared state with mutexes or, preferably, avoid sharing — collect results via a
  slice indexed per strategy or a channel drained after `Wait`.
- The SSE writer serializes chunk writes; do not write to the same `http.ResponseWriter`
  from multiple goroutines. Fan results into one channel, write from the handler goroutine.

## 7. Logging and output

- Structured logging via `log/slog` only. No `fmt.Println`/`log.Printf` in shipped code.
- Attach `trace_id` to logs (from the OpenTelemetry span context) — see Observability in
  the constitution.
- Log at boundaries (request in/out, LLM call, DB error), not in tight loops. Never log
  secrets or full scraped page bodies.

## 8. HTTP (chi) and generated code

- Handlers are thin: decode -> validate -> call use case -> map result/error to response.
  No business logic in handlers.
- Types and server stubs come from `oapi-codegen` (`internal/api`). Do not hand-edit
  generated files; change `openapi.yaml` at the umbrella root, `make sync-openapi`,
  regenerate.
- Validate input with `go-playground/validator` at the edge; return `400`/`422` Problem
  responses for invalid input.

## 9. Resource hygiene

- `defer` closes: HTTP response bodies, rows, files, the SSE flusher lifecycle.
- Check the error of deferred `Close()` where it matters (writes/flush), ignore where it
  does not (read bodies) with a clear reason.
- Set timeouts on outbound HTTP clients and DB pools; never rely on defaults.

## 10. Style and tooling

- Code MUST pass `golangci-lint` with zero warnings and be `gofumpt`-formatted.
- Exported identifiers have doc comments starting with the identifier name.
- Naming: short receivers, no stutter (`llm.Provider` not `llm.LLMProvider` in a package
  named `llm`), acronyms stay uppercase (`HTTPClient`, `ID`, `URL`).
- Prefer standard library; add a dependency only when it earns its place. The stack is
  fixed by the constitution — do not introduce alternatives (no Gin/Echo, no GORM).
- Keep functions small and single-purpose; extract when a function grows past one screen.

## Quick checklist before committing Go

- [ ] Correct layer; import direction respected (usecase→domain only, adapter never→usecase)
- [ ] Errors wrapped with `%w`; sentinels in domain; mapped to Problem at the HTTP edge
- [ ] `ctx` first param, propagated, cancellation respected
- [ ] Parallel work uses errgroup with context; no goroutine leaks
- [ ] slog structured logs with `trace_id`; no `fmt.Println`; no secrets logged
- [ ] Constructors inject deps; no globals; wiring only in `cmd`
- [ ] `golangci-lint` clean; gofumpt-formatted; exported docs present
- [ ] Tests written (see the `write-go-tests` skill)
