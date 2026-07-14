---
name: "start-task"
description: "Start a task the right way in pricing-optimizer-api: never develop on main. Use BEFORE writing any code for a new feature, fix, or chore. Ensures main is up to date, then creates the correctly named branch (NNN-slug via /speckit-specify for spec-driven features, or <type>/<slug> for standalone changes) so the work can later open a PR to main."
argument-hint: "Short description of the task, or the type + slug you want"
metadata:
  author: "rodvieira"
user-invocable: true
disable-model-invocation: false
---

# start-task — branch before development

The constitution forbids developing on `main` (Development Workflow & Quality Gates,
branch-per-task, NON-NEGOTIABLE). Run this before touching code for any task. Its job is to
put you on a correctly named, up-to-date branch. It does NOT write feature code.

## Step 0 — is this a spec-driven feature or a standalone change?

- **Spec-driven feature** (a real product capability that deserves a spec): do NOT hand-make
  a branch. Run `/speckit-specify "<feature description>"`. Spec Kit creates the
  `NNN-slug` branch (e.g. `002-generate-variations`) AND the matching `specs/NNN-slug/`
  directory. Renaming that branch breaks Spec Kit's feature detection, so leave it.
  After specify, continue with `/speckit-plan` → `/speckit-tasks` → `/speckit-implement`.
- **Standalone change** (tooling, CI, deps, docs, a small fix not warranting a full spec):
  create a `<type>/<slug>` branch as below.

If unsure, ask the user which it is before creating anything.

## Step 1 — verify a clean, current starting point

```bash
git status --short            # must be clean; stash or commit intentional WIP first
git switch main
git pull --ff-only origin main   # skip the pull if origin is not set up yet
```

Never branch off another feature branch. Always branch from an up-to-date `main`.

## Step 2 — create the standalone branch

Naming: `<type>/<slug>`
- `<type>` is a Conventional Commit type: `feat`, `fix`, `chore`, `refactor`, `docs`,
  `ci`, `build`, `perf`, `test`.
- `<slug>` is kebab-case, concise, describes the change (not the solution). Lowercase,
  ASCII, hyphen-separated, no trailing hyphen.

```bash
git switch -c <type>/<slug>
```

Examples:
- `ci/golangci-lint-workflow`
- `chore/lefthook-commitlint`
- `build/multi-stage-dockerfile`
- `fix/sse-flush-on-client-disconnect`
- `docs/backend-readme`

Slug rules: derive from the task description, drop filler words, keep it under ~40 chars.
One branch = one logical unit of work; if the task is really two concerns, make two tasks.

## Step 3 — confirm and hand off to development

```bash
git branch --show-current     # confirm you are on the new branch, not main
```

State the branch name back to the user, then proceed with development. Commits on this
branch follow the `commit` skill. When the work is ready, open a PR to `main`
(`gh pr create --base main`), run the `pr-reviewer`, and merge only once approved with CI
green — per the constitution.

## Guardrails

- If you are about to edit code and `git branch --show-current` says `main`, STOP and run
  this skill first.
- Do not create a branch for a spec-driven feature by hand — that is `/speckit-specify`'s job.
- Do not push directly to `main` or merge your own PR without the review + green CI gate.
