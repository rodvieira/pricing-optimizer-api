---
name: "commit"
description: "Craft Conventional Commits for pricing-optimizer-api that match commitlint and this project's scopes and hygiene rules. Use when staging and committing Go/backend changes: choose type + architecture-aligned scope, write an imperative English subject and a why-focused body, split unrelated changes into separate commits, and append the required Co-Authored-By trailer."
argument-hint: "Optional: a hint about what changed or the intended scope"
metadata:
  author: "rodvieira"
user-invocable: true
disable-model-invocation: false
---

# Commit crafting — pricing-optimizer-api

Produce clean, conventional commits. Commits are part of the portfolio: the history is
read by reviewers. Follow the constitution's Shipped-Artifact Discipline (English, no
emojis, Conventional Commits enforced by commitlint + lefthook).

## When to commit

- Commit ONLY when the user asks. Do not auto-commit after making changes.
- You MUST already be on a task branch, never `main` (constitution: branch-per-task). If
  `git branch --show-current` is `main`, stop and use the `start-task` skill first — a
  spec-driven feature branch is `NNN-slug` (from `/speckit-specify`), a standalone change
  is `<type>/<slug>`. Do not commit directly to `main`.
- Changes reach `main` only via a PR that is approved (pr-reviewer + green CI). Do not merge
  your own PR without that gate.
- Never commit secrets, `.env` files, generated coverage, or `tmp/` artifacts (the
  `.gitignore` already excludes these — verify `git status` before staging).

## Format

```
type(scope): imperative subject in English, <= 72 chars

Body: explain WHAT changed and, more importantly, WHY. Wrap at ~72 chars.
Reference the driver (an ADR, a sprint task, the constitution) when relevant.

BREAKING CHANGE: describe the contract/behavior break, if any.
Refs: #<issue>
```

- Subject: imperative mood ("add", not "added"/"adds"), no trailing period, no emoji,
  lowercase after the colon.
- Body is optional for trivial changes but expected for anything non-obvious — the WHY is
  what future readers need.
- Do NOT add a `Co-Authored-By: Claude ...` trailer or any AI attribution. Commits are
  authored solely by Rodrigo.

## Types

| type       | use for                                                              |
| ---------- | -------------------------------------------------------------------- |
| `feat`     | a new capability or endpoint behavior                                |
| `fix`      | a bug fix in shipped behavior                                        |
| `refactor` | code change that neither fixes a bug nor adds a feature              |
| `test`     | adding or changing tests only                                        |
| `docs`     | docs, README, ADRs, constitution, code comments only                 |
| `chore`    | tooling, deps, config, scaffolding with no product behavior change   |
| `ci`       | CI/CD workflow changes                                               |
| `build`    | build system, Dockerfile, Makefile                                   |
| `perf`     | a change that improves performance                                   |

## Scopes (aligned to the architecture)

Pick the scope from the layer/area the change lives in:

`domain`, `usecase`, `httpapi`, `scraper`, `llm`, `repository`, `cache`, `api` (generated),
`config`, `telemetry`, `db` (migrations/queries), `openapi` (contract), `deps`, `ci`,
`docker`, `test`. Omit the scope only when a change is genuinely cross-cutting.

## One concern per commit

- Split unrelated changes. A refactor plus a feature is two commits. Contract change plus
  its regenerated code plus handlers can be one commit only if they form a single logical
  unit (spec + `make sync-openapi` + regenerate).
- Review the staged diff before writing the message: `git diff --staged`. The message must
  describe what is actually staged — nothing more.
- Never use `git add -A` blindly; stage intentionally (`git add <paths>`).

## Workflow

1. `git status` and `git diff` (and `git diff --staged`) to see exactly what changed.
2. Group changes into logical commits; stage the first group intentionally.
3. Choose type + scope; write subject and body from the actual diff.
4. Verify: does the subject fit <= 72 chars, is it imperative English, is the scope right,
   and is there no AI/co-author attribution?
5. Commit. lefthook + commitlint run on commit; if they reject, fix and recommit — do not
   bypass hooks with `--no-verify`.

## Examples

```
feat(llm): add Groq provider behind LLMProvider port

Implements StreamStructured/GenerateStructured against Groq's
OpenAI-compatible API with the 8B fallback model. Selected by the
env-based factory per ADR-0003; no use case knows the concrete provider.
```

```
test(usecase): cover error aggregation in GenerateVariations

Adds table-driven cases for the first-error-cancels-siblings path and
per-strategy fan-out, bringing usecase coverage above the 80% gate.
```

```
chore(ci): add golangci-lint and test workflow on main
```
