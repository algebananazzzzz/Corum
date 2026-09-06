# Corum Go v2 Handover

Date: 2026-09-06 (Asia/Singapore)

## Current state

`main` at `140158c` contains the completed auth-flow restructure. The working
tree now contains the project-local configuration migration requested after
that commit.

The current change makes these additional decisions:

- `corum init [PATH]` is purely local and deterministic: no Jira prompt, no
  network, browser, OAuth, or credential work.
- `corum auth [PATH]` authenticates every enabled service: Canvas first
  (store token if none, validate against the configured origin, print the
  accessible courses with numeric IDs for `course.yaml` authoring), then Jira
  (forced browser OAuth, site/project selection, non-secret `jira` block in
  `.config/corum/corum.yaml`) when a `jira` block exists, otherwise an "Enable
  Jira now?" prompt.
- `corum auth jira [PATH]` / `corum auth canvas [PATH]` are the special
  individual flows. `corum jira login` is removed.
- `corum jira status [PATH]` / `corum jira logout [PATH]` take an optional
  vault path.
- All configuration is project-local under `<vault>/.config/corum/`:
  `corum.yaml`, `canvas.json`, and `auth.json`. Commands without a path use the
  current project; global configuration fallbacks are removed.
- Existing valid v2 root `corum.yaml` files migrate atomically on first open;
  v1 and ambiguous dual-path layouts are not modified.
- The global vault registry is removed. Toolkit refresh applies only to the
  project used by the current vault-aware command.
- Only the update-check cache remains global.

Current verification passes: `go test -count=1 ./...`, `go test -race -count=1
./...`, `go vet ./...`, `gofmt -l .`, `git diff --check`, `goreleaser check`, and
`goreleaser release --snapshot --clean`. All four snapshot archive checksums and
approved member lists pass; both Linux binaries are static and stripped.

Docs amended in the same change: the design spec (auth flow section),
README, SECURITY.md, and `agent-kit/AGENTS.base.md`.

## Remaining steps

1. Commit and publish only when explicitly requested.
2. The pre-existing global Jira cache is intentionally left untouched, but new
   code never reads or writes it. Re-authenticate within each project as needed.

## Standing constraints

- Work directly on local `main`; the user rejected worktrees.
- The design spec
  (`docs/superpowers/specs/2026-09-06-corum-go-v2-design.md`) is the authority;
  amend it in the same commit when behavior changes.
- Never print or inspect credential files. Leave any legacy global credential
  files intact unless the user separately authorizes their deletion.
- Releases are Linux/macOS × AMD64/ARM64 only (`CGO_ENABLED=0`); the
  `internal/lockfile` package carries a unix-only build tag by design.
- The `.superpowers/sdd/...` directory is git-ignored and is the execution
  ledger; preserve it and append entries as work lands.

## Process rulings carried forward

- One consolidated final gate (tests, race, vet, format, diff-check,
  goreleaser check + snapshot with archive verification) per change wave,
  instead of repeated per-task review/test cycles.
- Scoped final reviews over the wave diff, not fresh per-task reviews.
- Keep the clean-v2 boundary: no Python, no compatibility bridge.
