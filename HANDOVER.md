# Corum Go v2 Handover

Date: 2026-09-06 (Asia/Singapore)

## Stop state — read this first

The user stopped work because credits ran out. The final fix agent was interrupted.

**Do not reset, checkout, clean, or discard the working tree.** It contains a substantial uncommitted partial fix wave on top of the reviewed Go v2 cutover.

Repository: `/home/daniel/Documents/Corum`

Branch/state at handover:

```text
main at cbd1b52 feat!: replace Corum with clean Go v2
main is 10 commits ahead of origin/main
only the root checkout is registered; no linked worktrees
no push, GitHub release, or publish has occurred
```

Uncommitted partial fix files:

```text
 M internal/canvas/client_test.go
 M internal/canvas/placement.go
 M internal/canvas/state.go
 M internal/canvas/sync.go
 M internal/canvas/sync_test.go
 M internal/jira/apply.go
 M internal/jira/apply_test.go
 M internal/jira/models.go
 M internal/jira/state.go
 M schemas/run-manifest.schema.json
?? internal/lockfile/
?? HANDOVER.md
```

Before `HANDOVER.md`, the partial fix diff was approximately 746 insertions and 48 deletions. It has not been committed or fully tested. Some newly added tests are intentionally ahead of implementation and may fail.

## Product decisions

- Clean Go replacement; no Python runtime, compatibility bridge, or v1 migration.
- One `corum` binary for Linux/macOS, AMD64/ARM64, `CGO_ENABLED=0`.
- Huh v2 for interactive setup.
- Jira is OAuth-only through Atlassian Rovo MCP MCP. No Rovo CLI dependency, API-token fallback, keyring, or custom OAuth protocol.
- Authenticate at canonical `https://mcp.atlassian.com/v2/mcp`, then reuse the staged client/token source for `?tools=all` through a second official SDK handler.
- `version: 2`; service presence is the feature switch.
- Wiki prose is authored by LLM/agent skills, never synthesized/finalized by Go code.
- Corum owns and overwrites only `AGENTS.md`, `skills/`, `templates/`, and `.corum/toolkit-version` in registered vaults.
- Auto-update checks only when `corum` runs, at most once per 24 hours. `DISABLE_AUTO_UPDATES=1` disables automatic checks; explicit `corum update` bypasses the cache.
- No sandboxing, containers, server, database, daemon, scheduler, or multi-user hosting in v2.
- `/home/daniel/Projects/odyssey-cicd/` is the installer/GoReleaser/update/workflow reference.

Authorities:

- Design: `docs/superpowers/specs/2026-09-06-corum-go-v2-design.md`
- Plan: `docs/superpowers/plans/2026-09-06-corum-go-v2.md`
- Execution ledger: `.superpowers/sdd/2026-09-06-corum-go-v2/progress.md`
- Final gate report: `.superpowers/sdd/2026-09-06-corum-go-v2/task-7-report.md`
- Exact final review findings: `.superpowers/sdd/2026-09-06-corum-go-v2/final-review-findings.md`
- Full reviewed diff: `.superpowers/sdd/2026-09-06-corum-go-v2/review-67e3f8a..cbd1b52.diff`

The `.superpowers/sdd/...` directory is intentionally git-ignored. Preserve it until the work is finished.

## Completed commits

```text
b09e876 docs: design clean Go v2 replacement
2407530 docs: plan clean Go v2 replacement
3788e8a feat: establish Go OAuth and Rovo client
6315938 feat: add clean Go v2 vaults
4fbde4f fix: harden v2 vault validation
4429024 feat: add Huh setup and Jira selection
6fd34d6 feat: port safe Jira plan application
210a089 feat: port Canvas synchronization to Go
4225208 feat: ship and update the Corum binary
cbd1b52 feat!: replace Corum with clean Go v2
```

At committed head `cbd1b52`, the implementation agent reported these passing gates:

```text
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
git diff --check
goreleaser check
goreleaser release --snapshot --clean
```

It also verified four checksummed Linux/macOS AMD64/ARM64 archives, static/stripped Linux output, archive contents, installer simulations, update/checksum/re-exec behavior, daily skip/opt-out, and two-vault toolkit rollout. These results predate the current uncommitted fix wave and must be rerun after it is completed.

## OAuth checkpoint

Production OAuth passed before the final cutover:

- one successful browser consent;
- cache directory/file modes `0700`/`0600`;
- fresh-process `jira status`;
- forced access-token expiry and refresh with browser launch unavailable;
- subsequent cached reuse;
- account, accessible-resource, project, and project-bounded `maxResults: 1` search calls;
- no create/edit/transition calls.

Current Go cache: `/home/daniel/.config/corum/auth.json`.

The old Python-format cache was moved, not migrated, to:

```text
/home/daniel/.config/corum/auth.v1-python-backup.json
```

Both are credential files. **Do not print or inspect their contents.** Once the Go release is finished, ask before deleting the old backup. Several timed-out attempts may have created stale temporary Corum grants in Atlassian; recommend that the user revoke stale grants while keeping the working current grant.

## Why the committed head was not published

The single final whole-branch review rejected publication despite the passing gates. It found two Critical and thirteen Important correctness/security gaps. The exact report is in `final-review-findings.md`.

### Critical

1. Jira did not persist an exact `unknown` mutation-in-flight barrier before each remote write, so a crash could permit a duplicate retry.
2. Jira and Canvas lacked a shared cross-process per-course lock, so concurrent commands could duplicate writes or overwrite a newer reconciliation barrier.

### Important

1. Partial module changes could rebuild `modules.md` without unchanged modules.
2. Equivalent assignment due-time offsets could appear changed every run.
3. Canvas writes could escape through nested/target symlinks.
4. Canvas transport errors could expose verifier-bearing URLs.
5. Several source types omitted parity metadata needed by the agent toolkit.
6. `sync --json` did not emit complete per-course run manifests or preserve earlier course results on a later error.
7. Canvas state advanced before durable manifest persistence.
8. Explicit Jira login did not force account replacement; rollback errors could be discarded.
9. Optional vault paths/registry repair were incomplete for doctor/status/ordinary opens.
10. Explicit update re-executed `corum update`, causing a second metadata check and possibly incorrect final reporting.
11. The release workflow could tag main before a correctness gate.
12. Canvas/Jira pagination lacked repeat/progress bounds; Jira HTTP lacked a finite timeout.
13. Toolkit rollout lacked a per-vault lock and could delete another process's transaction files.

Minor findings: clear v1 guidance, registry update locking, and rune-safe slug truncation.

## Partial fix wave at interruption

The interrupted agent had made substantial but incomplete changes. No report or commit was produced.

Likely implemented or substantially implemented in the working tree:

- `internal/lockfile/`: Linux/macOS advisory file lock with a cross-process contention test.
- Jira per-course locking and exact pre-mutation `unknown` barrier, with crash-helper and concurrency tests.
- Canvas per-course locking.
- Full module rebuild from changed and unchanged modules.
- Canonical same-minute assignment due comparisons.
- Nested/target symlink rejection and safer placement.
- Restored normalized Canvas metadata and fixture-style tests.
- Manifest-before-state ordering and failure-order tests.
- `StageResult.Manifest`/run-manifest schema work.
- New Canvas tests for verifier redaction and bounded/repeated pagination.

Definitely still incomplete because the relevant production files were untouched:

- Canvas transport-error redaction and pagination implementation (`internal/canvas/client.go` unchanged; tests were added).
- Full CLI JSON accumulation/emission (`internal/cli/app.go` unchanged).
- Forced Jira reauthentication and joined rollback errors (`internal/jira/oauth.go`, `internal/ui/*` unchanged).
- Optional vault paths and automatic registry repair (`internal/cli/app.go` unchanged).
- Explicit-update continuation (`internal/update/*` and CLI unchanged).
- Release workflow verify dependency (`.github/workflows/3-release.yml` unchanged).
- Jira pagination bounds and finite HTTP timeout (`internal/jira/rovo.go`/OAuth client wiring unchanged).
- Per-vault toolkit lock and transaction-owned cleanup (`internal/vault/toolkit.go` unchanged).
- Registry read-modify-write lock (`internal/vault/registry.go` unchanged).
- Clear v1 diagnostic (`internal/config/*`/CLI unchanged).
- Rune-safe slug truncation (`internal/canvas/convert.go` unchanged).

Treat every partial item as unverified until its diff and tests are inspected.

## Recommended continuation

1. Read this file, the design, `final-review-findings.md`, and the current diff. Do not discard the partial work.
2. Run only the focused partial-wave tests first:

   ```bash
   go test -count=1 ./internal/lockfile ./internal/jira ./internal/canvas
   ```

3. Finish every Critical and Important item. Add/fix focused regressions before broad gates. Fix the three minors if still local and low risk.
4. Format and run the final gate once:

   ```bash
   gofmt -w <changed-go-files>
   go test -count=1 ./...
   go test -race -count=1 ./...
   go vet ./...
   git diff --check
   goreleaser check
   goreleaser release --snapshot --clean
   ```

5. Verify all four archives, checksums, approved archive members, and static Linux binary again.
6. Commit the fix wave, suggested subject:

   ```text
   fix: harden Corum v2 release boundaries
   ```

7. Do one scoped final review of `cbd1b52..HEAD`, focused on `final-review-findings.md`. Avoid restarting the entire implementation or repeating earlier per-task reviews.
8. If clean, confirm the root checkout is the only worktree and the tree is clean, then push `main` and observe the verify/tag/release workflow. No remote repository action has happened yet.

## Process rulings already made

- Work directly on local `main`; the user explicitly rejected additional worktrees.
- Keep the clean-v2 boundary; do not migrate the old Python OAuth cache.
- Favor the design authority over plan wording by using separate official SDK handlers for canonical bootstrap and all-tools token reuse.
- Toolkit Jira examples were intentionally deferred to Task 7 and are now updated.
- The user explicitly replaced per-task review/test repetition with one consolidated final gate to reduce time and tokens.
- After the final review, use one consolidated fix wave rather than multiple per-finding agents.

## External side effects

- Atlassian OAuth consent and read-only calls occurred as described above.
- No Jira mutation occurred.
- No Canvas live call occurred.
- No git push, tag, GitHub release, package publication, or workflow trigger occurred.
