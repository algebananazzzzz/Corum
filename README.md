# Corum

```console
curl -fsSL https://raw.githubusercontent.com/algebananazzzzz/Corum/main/install.sh | sh
corum init ~/Corum
```

Corum is a local-first framework for maintaining private academic vaults. The
single `corum` binary captures Canvas source material and applies exact approved
Jira plans. The installed agent toolkit owns semantic scoping and all wiki prose.

Corum stores no course content, credentials, Canvas identifiers, or Jira records
in this repository. Each initialized vault is a separate private artifact.

## Install

The installer supports Linux and macOS on AMD64 and ARM64. It downloads the
matching release archive and `checksums.txt`, verifies SHA-256, and installs the
binary as `${CORUM_INSTALL_DIR:-$HOME/.local/bin}/corum`. It does not use root or
edit shell profiles.

Users need neither Python nor Go. To choose another destination:

```console
curl -fsSL https://raw.githubusercontent.com/algebananazzzzz/Corum/main/install.sh |
  env CORUM_INSTALL_DIR="$HOME/bin" sh
corum version
```

Ensure the install directory is on `PATH`. Source contributors need the Go
version declared in `go.mod`, but the released binary has no language-runtime
prerequisite.

## Initialize a vault

Choose an empty path; initialization refuses to overwrite a nonempty target.

```console
corum init ~/Corum
corum auth ~/Corum
corum doctor ~/Corum
```

`corum init` is an interactive terminal wizard that is purely local and
deterministic. For automation or a minimal Canvas-first setup, use the headless
defaults:

```console
corum init --defaults ~/Corum
```

The vault receives `.config/corum/corum.yaml`, `.config/corum/.gitignore`,
`CLAUDE.md`, `skills/`, `templates/`, an empty `courses/` directory, and
`.corum/toolkit-version`. Corum keeps no global vault registry; release builds
refresh only the toolkit belonging to the project used by the current command.

`AGENTS.md` links to `CLAUDE.md`. Each of `.claude/skills`, `.codex/skills`, and
`.agents/skills` links to `../skills`. These relative links survive moving the
vault; toolkit refresh also repairs missing links in existing vaults.

Corum owns and may replace `CLAUDE.md`, these links, `skills/`, and `templates/`
paths plus `.corum/toolkit-version`. Keep personal instructions and files outside
those paths. Toolkit refreshes never replace `.config/corum/corum.yaml`,
`courses/`, captured sources, wiki pages, state, calendars, or credentials.

## Authentication

`corum init` is purely local: it writes project configuration and the toolkit.
Authentication is a separate interactive step:

```console
corum auth ~/Corum        # authenticate every enabled service
corum auth canvas ~/Corum # Canvas token only
corum auth jira ~/Corum   # Jira browser OAuth only
corum jira status [path]
corum jira logout [path]
```

The Canvas flow stores your API token in the vault, validates it against the
configured origin, and opens a searchable checklist containing only current,
identifiable courses. Existing tracked courses are preselected. Selecting a new
course creates its `course.yaml` with every Canvas source enabled; deselecting a
current course removes only its `canvas` block. Course files, captured data,
state, Jira, wiki, and tracked courses absent from the current Canvas response
are left untouched. For automation you can keep exporting
`CORUM_CANVAS_TOKEN` instead; it takes precedence over the stored token.

Jira uses Atlassian browser OAuth only. There is no email/API-token mode, Rovo
CLI dependency, keyring integration, or local protocol server. `corum auth
jira` opens Atlassian in the default browser, lets the user select an
accessible site and project, and writes only the non-secret `jira` block to
`.config/corum/corum.yaml`.

All Corum configuration is project-local: `<vault>/.config/corum/` holds
`corum.yaml`, `canvas.json`, and `auth.json`. The directory is private, and its
gitignore guard excludes the credential files. Commands without an explicit
path use the current project and never fall back to a global configuration
directory. `CORUM_CANVAS_TOKEN` remains the only credential override. Do not
copy, inspect, or commit credential files.

Disabled Jira paths and `corum jira apply ... --dry-run` do not open OAuth or make
Jira calls.

## Clean v2 configuration

Corum v2 accepts only `version: 2`. Service-block presence is the feature switch;
the v1 `features` mapping and `schema: 1` vocabulary are unsupported.
Workspace configuration lives at `.config/corum/corum.yaml`. The first command
that opens an existing v2 vault atomically moves a legacy root `corum.yaml` to
that path; if both files exist, Corum refuses the ambiguous vault.

```yaml
version: 2
workspace:
  timezone: Asia/Singapore
  term: AY2026/27 Semester 1
canvas:
  url: https://canvas.example.edu
jira:
  cloud_id: opaque-atlassian-cloud-id
  project: STUDY
  transitions:
    this_week: "2"
wiki: {}
calendar:
  timetable: Timetable.md
  term: Term_Calendar.md
```

Omit a service block to disable that service. Jira browser login writes only the
selected non-secret `cloud_id` and project. URLs must be credential-free HTTPS
origins, and `workspace.timezone` must be an IANA time zone.

Canvas authentication can create course configuration for you. You can also
create `courses/COURSE/course.yaml` manually with static course identity and the
service blocks enabled for that course:

```yaml
version: 2
code: COURSE
canvas:
  id: 1
  sources:
    - announcements
    - assignments
    - files
    - pages
    - modules
    - syllabus
  folders:
    Course Materials: lectures
jira:
  epic: STUDY-1
wiki:
  split_rules: default
```

A service operates only when its block is present in both workspace and course
configuration. Disabled services require no files, credentials, client setup, or
network access.

Version-1 vaults are rejected before mutation. Corum intentionally provides no
in-place compatibility layer or migration command: initialize a new v2 vault and
manually import only user-owned course and wiki content.

## Canvas capture and agent sync

Run deterministic capture from the vault root:

```console
cd ~/Corum
corum sync COURSE --json
corum sync COURSE_A COURSE_B --dry-run --json
corum sync --all
```

Capture writes successful sources below `courses/COURSE/raw/`, advances only
their Canvas state, and records the current run. Failed sources remain retryable
and are reported as unknown. The command does not invoke an LLM, write Jira, or
author wiki prose.

For the full workflow, ask an agent opened in the vault to “sync COURSE.” The
installed `sync-course` skill captures, scopes enabled Jira/wiki work, presents
one combined approval question, applies the exact approved Jira plan, and asks the
LLM to author and review approved wiki changes. Corum code never generates,
rewrites, or finalizes wiki prose.

## Exact Jira application

`corum jira apply` accepts a strict v2 JSON plan on standard input:

```console
corum jira apply COURSE --dry-run < plan.json
corum jira apply COURSE < approved-plan.json
```

Create, update, and transition actions form a closed union. Before the first
write, Corum validates the complete plan and proves every update or transition
target belongs to the configured epic. Writes are sequential, and partial or
uncertain results block replay until read-only reconciliation, fresh scoping, and
fresh approval make a new plan safe.

Jira operations use Atlassian's hosted Rovo MCP v2 endpoint through the official
MCP Go SDK. Normal tests use controlled local doubles and require no live
credentials.

## Updates

Release builds check for a newer version at most once every 24 hours when Corum is
invoked. A failed attempt is cached for the same interval so offline use does not
retry on every command. A verified update atomically replaces the binary,
re-executes the original command once, and refreshes the owned toolkit paths for
the active project when the requested command operates on a vault.

Set `DISABLE_AUTO_UPDATES=1` to disable automatic checks:

```console
DISABLE_AUTO_UPDATES=1 corum doctor ~/Corum
```

An explicit update always checks immediately and uses the same archive checksum
verification:

```console
corum update
```

Automatic-update failures are warnings and do not prevent the requested command
from running. See [SECURITY.md](SECURITY.md) for the binary, credential, data, and
toolkit trust boundaries.

## Product boundary

Corum has no hosted service, scheduler, database, daemon, sandbox, container
orchestration, keyring, Python bridge, Jira API-token fallback, or multi-user
account service. Each person runs one local binary under their operating-system
account and authorizes their own Atlassian account.
