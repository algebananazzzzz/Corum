# Corum

Corum captures Canvas course material into a local vault. An agent compares the captured changes with the course's items in your task tracker (Kaneo, Jira or Google Tasks) to find required work, sessions and milestones, and changes to items already in the tracker.

## Setup

```console
curl -fsSL https://raw.githubusercontent.com/algebananazzzzz/Corum/main/install.sh | sh
corum init
```

`corum init` is the whole first-time setup: vault path, academic term, Canvas URL and API token, the courses to track, and your task tracker (Kaneo, Jira, Google Tasks, or None). Kaneo and Jira ask for their URL. Init captures nothing yet.

Then open your agent in the vault and ask it to sync all your courses:

```console
cd path-to-vault
claude    # or codex
```

Corum does not sign in to task trackers. For Kaneo or Jira it installs the tracker's MCP server in `.mcp.json` and `.codex/config.toml`: approve and authenticate it with `/mcp` in Claude Code, or `codex mcp login <server>` in Codex. Google Tasks has no MCP server; agents use the Google Workspace CLI, so run `gws auth login -s tasks` once. The agent's first sync picks the project with you, captures each course in full, and proposes its plan.

`corum configure` changes a vault later: replace the Canvas token, add or drop courses, or switch trackers (`corum configure tracker`). Corum supports one tracker at a time; switching replaces the previous tracker's settings and does not migrate or delete remote tasks.

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

## Task trackers

The course code is the only mapping between a course and its tracker:

| Tracker | Course group | Type and categories | Due date |
| --- | --- | --- | --- |
| Kaneo | Label `CS3103` in one shared project | Labels | Exact local deadline or start time |
| Jira | Epic with summary `CS3103` | Issue type and labels | Date; a task due before 1500 uses the preceding date |
| Google Tasks | Task list titled `CS3103` | `Labels:` line at the end of the notes | Date |

Item titles start with `[CS3103]` in every tracker. Types are `task`, `session` and `milestone`.

On its first sync the agent completes setup: it lists your Kaneo or Jira projects, proposes one, and proposes a course group for each tracked course, including renames for misnamed ones. Approved choices are saved in `.config/corum/corum.yaml`:

```yaml
task_tracker: kaneo
kaneo:
  url: https://kaneo.example.com
  project: TOD
```

Jira saves `jira.site` (for example `https://your-team.atlassian.net`) and `jira.project`. Google Tasks needs no settings.

Ask for one course ("sync CS3103") or all of them; the agent syncs courses one at a time, each with a numbered plan you approve in full or by row. Every sync also creates recurring sessions through the end of next week's Friday and moves to-do items due by then into This Week (Kaneo's `This Week` column, or a Jira `This Week` status).

The installed `sync-course` skill keeps the course's items in `courses/<course>/state/tracker.json`, one format for every tracker. The agent reads the whole course only on the first sync or when you ask it to refresh; after that it updates the file from its own writes and reads each item by ID just before changing it. Approved writes go through the tracker's MCP server or `gws`.

## Maintenance

- `corum doctor [PATH]` validates workspace and course configuration and each course's `state/tracker.json`, printing what is wrong.
- `corum toolkit update [PATH]` explicitly refreshes bundled agent instructions
  and skills. It also removes retired bundled files: the three wiki authoring skills, the separate `scope-course` skill (now part of `sync-course`) and the old error-handling reference.
  Custom skills, existing course pages and your section at the end of
  `AGENTS.md` are preserved.
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

Workspace settings keep the selected timezone (SGT / `Asia/Singapore` by default) and academic term, the Canvas URL, and the selected task tracker's location. The selected term determines the bundled calendar installed as `Term_Calendar.md` for week lookup. Calendar paths and Jira transition mappings are not configurable.

Course configuration maps the course to Canvas; its code names it in the tracker. Canvas keeps only its comparison state; the agent keeps `state/tracker.json`. See [schemas/README.md](schemas/README.md) for the stored shapes.
