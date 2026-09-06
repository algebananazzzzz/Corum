# Corum Go v2 Design

## Goal

Replace Corum's Python implementation with one statically linked Go binary for
Linux and macOS. Corum v2 keeps the product's local-first academic workflow,
Jira safety boundaries, and LLM-owned wiki authoring while simplifying
installation, configuration, terminal setup, releases, and toolkit updates.

This is a clean replacement, not a compatibility migration. Git history remains
the reference for v1 behavior during development, but the released product
contains no Python runtime or fallback.

## Decisions

- Ship one Go binary named `corum`.
- Support Linux and macOS on AMD64 and ARM64.
- Use Huh for interactive terminal forms and its accessible prompt mode.
- Use the official MCP Go SDK for Atlassian Rovo OAuth and tool calls.
- Use GoReleaser and GitHub Releases, following the established patterns in
  `/home/daniel/Projects/odyssey-cicd`.
- Check for releases at most once every 24 hours, triggered only when `corum` is
  invoked. No daemon or scheduler is added.
- `DISABLE_AUTO_UPDATES=1` disables automatic release checks. `corum update`
  always performs an explicit check.
- Corum owns and overwrites each vault's `AGENTS.md`, `skills/`, and
  `templates/` trees. It never overwrites user configuration, course data, wiki
  pages, run state, or credentials.
- Do not add sandboxing, containers, a server, a database, keyring integration,
  multi-user hosting, or API-token Jira authentication.

## Command Surface

Corum v2 exposes:

```text
corum init [path]
corum doctor [path]
corum auth [path]
corum auth jira [path]
corum auth canvas [path]
corum sync [course...] [--all] [--dry-run] [--json]
corum jira status [path]
corum jira logout [path]
corum jira apply COURSE [--dry-run]
corum update
corum version
```

Interactive commands require a terminal. Deterministic, noninteractive paths
remain available for automation. Commands must emit concise user errors without
credentials, OAuth callback parameters, raw remote payloads, or stack traces.

### Authentication flow

`corum init` is purely local and deterministic: it writes
`.config/corum/corum.yaml` and the toolkit, and performs no network, browser,
or credential work.

`corum auth` authenticates every service enabled in the vault. The Canvas
flow stores the API token (prompting once when none is stored), validates it
against the configured origin, and prints the accessible courses with their
numeric IDs for `course.yaml` authoring. The Jira flow runs a fresh browser
OAuth against the configured vault, lets the user pick site and project, and
updates only the non-secret `jira` block in `.config/corum/corum.yaml`.
`corum auth jira`
and `corum auth canvas` run the individual flows.

All configuration is project-local: `<vault>/.config/corum/` holds
`corum.yaml`, `canvas.json`, and `auth.json`. The directory is `0700`, the
credential files are `0600`, and its gitignore excludes credentials. Commands
without an explicit path use the current project; there is no global
configuration fallback. `CORUM_CANVAS_TOKEN` still takes precedence for
automation.

## Repository and Package Structure

```text
cmd/corum/                 entry point and version injection
internal/cli/              command parsing and orchestration
internal/ui/               Huh prompts and accessible terminal mode
internal/config/           v2 YAML models and validation
internal/vault/            initialization, doctor, toolkit sync
internal/canvas/           Canvas HTTP capture and content conversion
internal/jira/             OAuth, Rovo adapter, plans, cache, reconciliation
internal/update/           release checks, verification, self-replacement
agent-kit/                 embedded agent instructions, skills, templates
schemas/                   embedded v2 JSON schemas
```

`cmd/corum` embeds `agent-kit/` and `schemas/` with `go:embed`. Internal packages
own behavior; the entry point only wires dependencies and maps errors to exit
codes. External service clients are injected behind small interfaces so tests
use local HTTP and MCP doubles.

The final cutover removes `pyproject.toml`, `src/corum/`, Python tests, and Python
installation instructions. No dual-runtime release is produced.

## Clean v2 Configuration

The project configuration is `.config/corum/corum.yaml` with `version: 2`.
Service presence is the feature switch; v1's separate `features` mapping is
removed. A valid legacy v2 root `corum.yaml` is atomically moved on first open;
v1 files are not moved, and dual paths are rejected as ambiguous.

```yaml
version: 2
workspace:
  timezone: Asia/Singapore
  term: AY2026/27 Semester 1
canvas:
  url: https://canvas.example.edu
jira:
  cloud_id: "opaque-atlassian-cloud-id"
  project: TODO
  transitions:
    this_week: "2"
wiki: {}
calendar:
  timetable: Timetable.md
  term: Term_Calendar.md
```

`jira`, `canvas`, and `wiki` are enabled when their corresponding configuration
exists. Course configuration uses the same presence rule. Identifiers, issue
keys, origins, time zones, relative paths, and transition IDs retain strict
validation. Configuration remains credential-free.

State documents use `version: 2` and atomic replacement. A v1 vault produces a
clear unsupported-version error and is not modified. There is no automatic or
in-place migration command. Users initialize a new v2 vault and manually import
their course/wiki content.

## Vault Initialization and Toolkit Ownership

`corum init` uses Huh to collect the vault path, time zone, term, Canvas
origin, and wiki choice. It is purely local and deterministic: no network,
browser, OAuth, or credential work happens during initialization, and no Jira
step is offered. The final summary contains no credentials. Nothing is written
before confirmation, and initialization refuses a nonempty target. Jira is
configured later by `corum auth` after a `jira` block exists in
`.config/corum/corum.yaml`.

The binary embeds the canonical toolkit. Initialization writes:

```text
.config/corum/
  .gitignore
  corum.yaml
AGENTS.md
skills/
templates/
courses/
.corum/toolkit-version
```

Toolkit synchronization stages a complete new `AGENTS.md`, `skills/`, and
`templates/` set beside the existing files and replaces only those owned paths.
The version marker changes only after all replacements succeed. User files are
outside this boundary. There is no registry: release startup refreshes only the
valid project targeted by the current vault-aware command. Failures are warned
without blocking command dispatch.

## Jira OAuth and Rovo MCP

Jira remains OAuth-only and targets Atlassian's hosted Rovo MCP v2 service. The
production implementation uses the successful Go feasibility result:

1. Bind an ephemeral loopback callback on `127.0.0.1`.
2. Authenticate through the canonical
   `https://mcp.atlassian.com/v2/mcp` resource URL.
3. Let the official Go SDK own discovery, dynamic client registration, PKCE,
   issuer/state validation, code exchange, scope handling, and refresh.
4. Reuse the resulting token source for
   `https://mcp.atlassian.com/v2/mcp?tools=all`.
5. Paginate the complete tool catalog before requiring Corum's Jira tools.

Direct authorization through the query-bearing URL is forbidden because the Go
SDK currently treats its query string as part of the issuer and rejects
Atlassian's canonical metadata. The two-session sequence uses only official SDK
interfaces and contains no custom OAuth protocol implementation.

The portable cache lives under the platform user config directory, outside every
vault. It persists the dynamic client information, OAuth endpoint data, scopes,
and tokens needed for restart and refresh. POSIX directories use mode `0700` and
files use `0600`; writes are atomic. Unsafe ownership, unsafe permissions,
corruption, revocation, or missing refresh material produces a redacted login
error. Login replacement restores the previous cache if the new flow is
cancelled or fails.

The Rovo adapter exposes only account lookup, accessible resources, project
listing, issue reads/searches, and the create/edit/transition operations used by
Corum. It normalizes Rovo's compact wrappers and appended text footer at this
boundary. Tool errors never expose raw responses or tokens.

## Jira Mutation Safety

The Go implementation preserves these invariants rather than simplifying them:

- Plans are strict, versioned, and target one configured course and epic.
- Dry-run never opens OAuth or performs a Jira call.
- The exact approved plan is the only input to application.
- Writes are sequential.
- Created keys and all remote response shapes are validated.
- Per-action results classify writes as not applied, applied, or unknown.
- Partial or uncertain writes block further nonempty plans.
- Recovery requires read-only reconciliation, fresh scoping, and fresh approval.
- Cache ownership checks prevent Corum from adopting unrelated Jira issues.

No automatic retry may duplicate a potentially applied mutation.

## Canvas and Wiki Boundaries

Canvas remains a direct HTTPS client authenticated with the project-local
credential store (environment variable first, then
`<vault>/.config/corum/canvas.json`).
The Go port preserves origin restrictions, pagination, source selection, safe
path placement, verifier-bearing URL stripping, conversion behavior, dry-run,
atomic state, and partial-failure reporting. HTML-to-Markdown and PDF extraction
libraries are selected only after fixture parity checks and must work with
`CGO_ENABLED=0`.

Go code captures and places source material but does not semantically author wiki
pages. Embedded agent skills own scoping, synthesis, diagram creation, wiki
linting, and the combined approval workflow. There is no code-driven "wiki
finalization" behavior beyond validating machine-readable state boundaries.

## Installation and Releases

GoReleaser builds stripped `CGO_ENABLED=0` archives for:

```text
linux/amd64
linux/arm64
darwin/amd64
darwin/arm64
```

Version information is injected with linker flags. Tagged releases produce
platform archives and `checksums.txt`. GitHub Actions adapts Odyssey's checkout,
semantic-version tag, Go setup, and GoReleaser jobs, using current stable action
majors and repository-scoped `contents: write` permission.

`install.sh` follows Odyssey's installer shape: detect OS and architecture, query
the latest GitHub Release, download the matching archive and checksums, verify
SHA-256 using the platform tool, extract, and install mode `0755` to
`${CORUM_INSTALL_DIR:-$HOME/.local/bin}`. It never uses root and reports when the
directory is absent from `PATH`.

## Automatic Updates

The update-check cache is stored in the platform user cache directory and
contains only the last check-attempt time and latest observed version. Recording
failed attempts prevents an offline machine from retrying on every invocation.
At process start:

1. Development builds and `DISABLE_AUTO_UPDATES=1` skip the check.
2. A cache younger than 24 hours skips network access.
3. Otherwise Corum queries the latest GitHub Release with a short timeout.
4. Network or decoding failure is nonfatal and the requested command continues.
5. A newer release triggers archive/checksum download and verification.
6. Corum atomically replaces its executable and re-executes the original command
   with an internal guard preventing an update loop.
7. For vault-aware commands, the new process synchronizes only the active
   project's toolkit before running the requested command.

Checksum failure, unsupported platform, unwritable executable, or extraction
failure leaves the installed binary and toolkits unchanged and emits one concise
warning. Concurrent processes use a best-effort update lock so only one performs
replacement; other invocations continue with the installed version.

`corum update` bypasses the 24-hour cache, performs the same verified replacement,
and reports whether the installed version changed. It does not require a
confirmation prompt because automatic updates are already enabled by default.

## Development Sequence

Implementation proceeds risk-first:

1. Establish the Go module, entry point, embedded assets, and test helpers.
2. Productionize the proven Jira OAuth cache, callback, canonical bootstrap, and
   `tools=all` connection.
3. Define clean v2 configuration and initialization with Huh.
4. Port Jira plan validation, adapter mappings, sequential application, cache,
   and reconciliation.
5. Port Canvas capture, conversion, placement, and state behavior.
6. Add active-vault transactional toolkit synchronization, installer,
   automatic updater, GoReleaser, and GitHub Actions.
7. Remove Python code and packaging, update documentation, and run acceptance.

Git history is used for behavioral reference; no long-lived duplicate
implementation or compatibility layer is added. Focused package checks run while
each area is built. The complete regression and release matrix is deferred until
the final integration stage to reduce review overhead.

## Verification and Acceptance

Automated gates are:

```text
go test ./...
go test -race ./...
go vet ./...
goreleaser check
goreleaser release --snapshot --clean
```

Tests use temporary directories and local HTTP/MCP servers. They cover strict v2
validation, no-write cancellation, path safety, OAuth-cache permissions and
redaction, Rovo normalization, all Jira operation mappings, mutation uncertainty,
reconciliation, Canvas fixtures, update intervals and opt-out, release selection,
checksum rejection, executable replacement, active-project toolkit refresh, and
transactional toolkit replacement.

Final installed-binary acceptance covers:

- Linux AMD64 archive installation through `install.sh`.
- Initialization and `doctor` on a new v2 vault.
- Automatic-update skip, daily-check, failure fallback, and
  `DISABLE_AUTO_UPDATES=1`.
- Toolkit overwrite across multiple registered test vaults without changes to
  user-owned files.
- User-assisted Atlassian login, process restart, forced token refresh, complete
  tool discovery, project selection, and bounded read-only Jira reconciliation.
- Jira dry-run proving no OAuth or Jira access.

Live Jira mutation occurs only with separate explicit approval of the exact plan
and target project. The port is complete only after Python is absent from the
release, the repository is clean, and the pushed `main` commit passes the release
and installed-binary gates.
