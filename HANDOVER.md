# Corum Go v2 Handover

Date: 2026-09-06 (Asia/Singapore)

## Current state

`main` at `c6ee852` is published as **v0.1.0** (tag + GitHub release with all
assets). The interrupted fix wave from the previous handover is fully
completed, released, and its history lives in
`.superpowers/sdd/2026-09-06-corum-go-v2/progress.md`.

The working tree on `main` contains the **auth flow restructure** (uncommitted
at the time of writing). It is complete and locally gate-verified:

- `corum init [PATH]` is purely local and deterministic: no Jira prompt, no
  network, browser, OAuth, or credential work.
- `corum auth [PATH]` authenticates every enabled service: Canvas first
  (store token if none, validate against the configured origin, print the
  accessible courses with numeric IDs for `course.yaml` authoring), then Jira
  (forced browser OAuth, site/project selection, non-secret `jira` block in
  `corum.yaml`) when a `jira` block exists, otherwise an "Enable Jira now?"
  prompt.
- `corum auth jira [PATH]` / `corum auth canvas [PATH]` are the special
  individual flows. `corum jira login` is removed.
- `corum jira status [PATH]` / `corum jira logout [PATH]` take an optional
  vault path.
- Credentials are **project-local by default**:
  `<vault>/.config/corum/canvas.json` and `auth.json`, `0700` directory,
  `0600` files, and a `*`/`! .gitignore` guard so they are never committed.
  The platform user configuration directory (normally `~/.config/corum/`) is
  used only when a command runs without a vault context.
  `CORUM_CANVAS_TOKEN` still takes precedence for automation.

Gate passed on the working tree: `go test -count=1 ./...`,
`go test -race -count=1 ./...`, `go vet ./...`, `gofmt -l .` clean,
`git diff --check` clean, `goreleaser check`, `goreleaser release --snapshot
--clean` with all four archives checksum-verified, approved member lists, and
static/stripped Linux AMD64/ARM64 binaries.

Docs amended in the same change: the design spec (auth flow section),
README, SECURITY.md, and `agent-kit/AGENTS.base.md`.

## Remaining steps

1. Commit the working tree with a `feat:` subject (e.g.
   `feat: add corum auth with project-local credentials`). The pre-1.0 release
   script (`.github/scripts/semver-tag.sh`) maps `feat:` to a minor bump, so
   the next tag is **v0.2.0**.
2. Push `main` and watch the `Release` workflow: verify gate → tag → publish.
   Confirm the new tag points at the pushed head and the release has all five
   assets (four archives + checksums).
3. Note for the user: the live Jira OAuth grant currently sits in the global
   fallback (`~/.config/corum/auth.json`). Existing vaults get their
   project-local cache automatically on the next `corum auth jira`; nothing is
   migrated.

## Standing constraints

- Work directly on local `main`; the user rejected worktrees.
- The design spec
  (`docs/superpowers/specs/2026-09-06-corum-go-v2-design.md`) is the authority;
  amend it in the same commit when behavior changes.
- Never print or inspect credential files. The live grant at
  `~/.config/corum/auth.json` must stay intact.
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
