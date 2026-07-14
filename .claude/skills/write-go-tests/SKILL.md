---
name: "write-go-tests"
description: "Write meaningful Go unit and integration tests for pricing-optimizer-api that verify behavior, not implementation. Use when adding or changing any Go code that needs tests: table-driven style with testify, generated uber-go/mock mocks, testcontainers-go for real Postgres, golden fixtures for LLM adapters, and the constitution's >=80% coverage gate on usecase and domain."
argument-hint: "Optional: the package, file, or function to test"
metadata:
  author: "rodvieira"
user-invocable: true
disable-model-invocation: false
---

# Writing Go tests — pricing-optimizer-api

Tests here exist to prove behavior and catch regressions, not to inflate coverage. Follow
the constitution's Test-First Rigor principle. Read the code under test first, identify its
observable behavior and failure modes, then write tests against those — through the
public/port surface, not private details.

## What to test (and what not to)

- Test BEHAVIOR through the exported API or the domain port, not internal helpers.
- Prioritize: the happy path, every error branch, boundary values, and the contract of each
  port (what a use case guarantees to its caller).
- For `usecase`: orchestration logic with all ports mocked — verify it calls the right ports
  with the right args, aggregates results, and maps failures correctly.
- For `domain`: pure business rules and invariants (pricing strategy constraints, validation).
- Do NOT test generated code, trivial getters, or the standard library.
- Do NOT assert on log lines or unexported fields. If you must reach internals to test
  something, that is usually a design smell — prefer refactoring toward a port.

## Structure: table-driven, always

Every test for a function with more than one case is table-driven with `t.Run`:

```go
func TestGenerateVariations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     domain.GenerateRequest
		setup   func(m *mockports.MockLLMProvider)
		want    int // number of variations
		wantErr error
	}{
		{
			name: "generates one variation per requested strategy",
			req:  fixtureRequest(domain.StrategyAnchor, domain.StrategyFreemium),
			setup: func(m *mockports.MockLLMProvider) {
				m.EXPECT().
					GenerateStructured(gomock.Any(), gomock.Any()).
					Return(fixtureVariation(), nil).
					Times(2)
			},
			want: 2,
		},
		{
			name: "returns the first provider error and cancels siblings",
			req:  fixtureRequest(domain.StrategyAnchor, domain.StrategyFreemium),
			setup: func(m *mockports.MockLLMProvider) {
				m.EXPECT().
					GenerateStructured(gomock.Any(), gomock.Any()).
					Return(nil, domain.ErrProviderUnavailable).
					AnyTimes()
			},
			wantErr: domain.ErrProviderUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			provider := mockports.NewMockLLMProvider(ctrl)
			if tt.setup != nil {
				tt.setup(provider)
			}

			uc := usecase.NewGenerateVariations(provider)
			got, err := uc.Execute(context.Background(), tt.req)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got.Variations, tt.want)
		})
	}
}
```

Rules shown above, all mandatory:
- Subtest names are descriptive English sentences of the behavior.
- `require` for preconditions that make the rest of the test meaningless if they fail
  (`require.NoError`, `require.ErrorIs`); `assert` for independent value checks.
- `t.Parallel()` on both the outer test and each subtest when there is no shared mutable
  state. Capture loop vars correctly (Go 1.22+ makes this safe, but keep tests self-contained).
- Compare errors with `require.ErrorIs` / `errors.As` — never string-match error messages.

## Mocks: generated only

- Mocks come from `uber-go/mock` (`go.uber.org/mock/gomock` + `mockgen`). NEVER hand-write a
  mock.
- Put a `//go:generate mockgen` directive next to each port in `domain`, e.g.:

```go
//go:generate mockgen -source=ports.go -destination=../../test/mocks/ports_mock.go -package=mockports
```

- Regenerate with `go generate ./...`. Assert interactions with `.EXPECT()`, `.Times(n)`,
  and matchers (`gomock.Any()`, `gomock.Eq(...)`). Use `gomock.NewController(t)` — it
  auto-verifies expectations on cleanup.
- Prefer verifying the arguments passed to ports (via matchers or `DoAndReturn`) over
  loosely allowing any call, when the arguments are part of the behavior.

## Integration tests: real Postgres via testcontainers

- Repository/adapter integration tests use `testcontainers-go` to spin a real Postgres —
  never a DB mock. File suffix `integration_test.go`, and guard slow suites behind a build
  tag or `testing.Short()`:

```go
func TestGenerationRepo_SaveAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()
	pg := startPostgres(t, ctx)      // helper: starts container, runs goose migrations
	repo := repository.New(pg.Pool)
	// ... exercise real SQL, assert round-trip ...
}
```

- Run goose migrations against the container before exercising queries so tests match prod
  schema. Use `t.Cleanup` (or the container's `Terminate`) to tear down.

## LLM adapter tests: mocked client + golden fixtures

- Test `adapter/llm` by injecting a fake HTTP/SDK client that replays recorded responses
  from `test/fixtures/` (golden files). Do not call real Groq/Anthropic in tests.
- Cover: correct request/tool-schema construction, successful structured parse, malformed
  response handling, and provider-error mapping to domain errors.
- Update golden files intentionally (e.g. via a `-update` flag), never by hand-editing to
  make a test pass.

## Helpers and hygiene

- Test helpers call `t.Helper()` as their first line so failures point at the caller.
- Build reusable fixtures (`fixtureRequest`, `fixtureVariation`) instead of duplicating
  setup; keep them in the test package or `test/fixtures`.
- Use `t.Cleanup` for teardown instead of manual defers scattered across the test.
- One logical behavior per subtest. If an assertion needs a paragraph to explain, split it.

## Coverage gate

- `internal/usecase` and `internal/domain` MUST stay at >= 80% coverage. Check locally:

```bash
go test ./... -short -coverprofile=coverage.out
go tool cover -func=coverage.out | tail -1
```

- Coverage is a floor, not the goal. A green 80% with no error-path assertions is a failure
  of intent. Make sure the meaningful branches are the ones covered.

## Checklist before finishing a test file

- [ ] Table-driven with descriptive English subtest names
- [ ] `require` for preconditions, `assert` for independent checks
- [ ] Errors compared with `ErrorIs`/`As`, never string match
- [ ] Mocks generated by uber-go/mock; expectations verified via `gomock.NewController(t)`
- [ ] Integration tests use testcontainers Postgres + real migrations, guarded by `-short`
- [ ] LLM tests use golden fixtures, no live API calls
- [ ] Helpers use `t.Helper()`; teardown via `t.Cleanup`
- [ ] Error paths and boundaries covered, not just the happy path
- [ ] usecase/domain coverage >= 80%
