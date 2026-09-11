# Corum

Corum captures Canvas course material and reads Jira epic issues into local
caches. An agent uses the captured changes and cached issues to identify required
work, sessions, milestones and updates to existing obligations.

## Setup

```console
curl -fsSL https://raw.githubusercontent.com/algebananazzzzz/Corum/main/install.sh | sh
corum init
cd path-to-vault
corum configure
```

The first `corum configure` guides you through Canvas connection, course selection,
task tracker selection (Google Tasks, Jira, or None), course destinations, and an
initial sync. Subsequent runs open a settings menu. Each section returns to that
menu when finished; valid connections are reused. Select **Sign in again / switch
account** to explicitly reconnect. Canvas uses an API token; setup explains where
to create it. The Tracked courses section reuses the saved token.

The mapping menu shows **Jira Epic mapping** or **Google Task List mapping**, with
the number of tracked courses mapped. Select a course, then choose an existing
destination (`/` searches), create one with a typed name, or remove its mapping.
Wide terminals show course mappings beside the destination form. Every successful
action saves immediately. Back discards only the unsubmitted form. Failed actions
report an error and preserve existing mappings. Remote creation followed by a
local save failure reports the destination ID; select that destination on retry.

Changing trackers retains old mappings and does not migrate or delete remote tasks.
The selected tracker is stored as `task_tracker`; no draft or setup-progress file
is created. Existing Jira configurations open the settings menu directly.
Jira MCP entries are installed for Codex and Claude; those clients may require
their own authentication, separately from Corum's connection.

Use `corum init --defaults PATH` for noninteractive initialization. Canvas defaults
to NUS; edit `.config/corum/corum.yaml` for another Canvas URL or timezone. Set
`CORUM_CANVAS_TOKEN` for automation, or save a token with `corum configure canvas`.

## Canvas course sync

```console
corum sync CS101 --json
corum sync CS101 CS102 --json
corum sync --all --json
```

Capture includes announcements, assignments, files, pages, modules and syllabus.
Text is converted to Markdown with source metadata; files are downloaded into
`courses/<course>/raw/`. Each course's `course.yaml` selects source types and
optional folder mappings.

JSON output is an array of per-course changesets:

```json
[
  {
    "course": "CS101",
    "dry_run": false,
    "status": "changed",
    "changes": [
      {
        "id": "canvas:announcements:1",
        "source": "announcements",
        "item_id": "1",
        "kind": "announcement",
        "status": "changed",
        "summary": "announcement 1 · Assessment briefing",
        "raw_path": "announcements/assessment-briefing-1.md",
        "details": {"title": "Assessment briefing"}
      }
    ],
    "failures": [],
    "sources": [
      {"source": "announcements", "status": "changed", "changes": ["canvas:announcements:1"], "failures": []}
    ]
  }
]
```

Consume stdout in the agent workflow. Corum keeps the capture comparison cache
at `state/canvas.json`, but does not save changesets, run manifests or changelogs.
A later sync may report no changes for material already captured. Partial failures
return a nonzero exit code while preserving available results in stdout.

`--dry-run` reports readiness without fetching Canvas data. Current incremental
capture recognizes new announcements/files, assignment due-date changes, page
updates, module changes and syllabus changes. It does not comprehensively detect
content-only edits or deletions for every source type.

## Jira epic sync

Add an existing epic to `courses/CS101/course.yaml`:

```yaml
jira:
  epic: STUDY-1
```

Then run:

```console
corum jira sync-epic CS101
```

Corum validates the epic, fetches its children and replaces `state/jira.json` with
the complete issue snapshot. If fetching or validation fails, the previous cache
is preserved. It does not create epics or write Jira issues.

Agents can propose changes using the installed `sync-course` and `scope-course`
skills. Approved writes go through the Jira MCP tools configured by Corum.
`corum configure jira [PATH]` opens task tracker settings. Use the main configuration
menu to change sites/projects or manage course mappings using your existing login.

The cache currently uses Jira issue fields and provider-specific issue types.

## Google Tasks sync

Use `corum configure` to connect Google and select or create course task lists.
For manual configuration, add a Google Tasks list to `courses/CS101/course.yaml`:

```yaml
google_tasks:
  list_id: '@default'
```

Then run:

```console
corum google sync-tasks CS101
```

Corum invokes the Google-maintained Workspace CLI (`gws`) and atomically writes
the normalized snapshot to `state/google-tasks.json`. Tasks with due dates appear
in Google Calendar automatically; the sync stores date-level granularity only.
Corum reuses an installed `gws`, or obtains pinned version 0.22.5 through `npx`
when Node.js is available. Set `CORUM_GWS_BIN` to use a specific CLI binary path.
This release uses the CLI JSON interface; that version has no MCP subcommand.

Corum does not yet distribute a registered Google OAuth client. On first Google
connection, setup opens the relevant Google Console pages and guides you through
project selection, enabling Tasks, consent settings, and downloading a Desktop
OAuth client JSON. Corum imports that file with private permissions, launches
Tasks-only login, and checks access. Existing Google credentials are reused.
Account consent and any organization administrator approval remain user actions.
Credentials stay in the Google CLI credential store, outside course files.
This developer-client preparation is temporary until a Corum-owned OAuth app is
registered and cleared for distribution.

## Maintenance

- `corum doctor [PATH]` validates workspace and course configuration.
- `corum toolkit update [PATH]` explicitly refreshes bundled agent instructions
  and skills. It also removes the three retired bundled wiki authoring skills.
  Custom skills and existing course pages are preserved.
- `corum version` prints the installed version.
- `corum update` explicitly checks for a newer release, verifies its SHA-256
  checksum and replaces the installed executable. Development builds cannot
  self-update; use the installer to install a release first.
- There are no startup update checks, background maintenance, update caches,
  toolkit version registries or runtime lockfiles.

Run one sync per course at a time. Configuration and cache formats are unstable;
there are no format versions, migrations or compatibility adapters.

Wiki authoring and the original combined scoping skill are preserved under
[`archive/skills/`](archive/skills/). They are not embedded or installed. Existing
wiki pages in user vaults are not deleted. See [SECURITY.md](SECURITY.md) for
credential and filesystem boundaries.

## Development

```console
gofmt -w cmd internal
go vet ./...
go test -race ./...
CGO_ENABLED=0 go build ./cmd/corum
```

## Minimal stored state

Workspace settings keep the selected timezone (SGT / `Asia/Singapore` by default)
and academic term, plus configured Canvas/Jira connection details. The selected
term determines the bundled calendar installed as `Term_Calendar.md` for week
lookup. Calendar paths and Jira transition mappings are not configurable.

Course configuration maps the course to Canvas and optionally a Jira epic.
Canvas keeps only its comparison state; Jira keeps the current issue snapshot.
See [schemas/README.md](schemas/README.md) for the stored shapes.
