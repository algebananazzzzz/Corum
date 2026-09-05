# Corum

Corum is a content-free framework for maintaining private academic vaults. Its
Python CLI captures Canvas sources and applies exact approved Jira plans;
vault-installed agent skills own semantic scoping, one approval gate, and optional
Obsidian wiki authoring.

Corum stores no course content, credentials, Canvas identifiers, or Jira records in
this repository. A user's initialized vault is a separate private artifact.

## Install

Corum requires Python 3.12 or newer. From a source checkout:

```console
python -m venv .venv
.venv/bin/pip install .
.venv/bin/corum --help
```

For development and tests, install the declared extra:

```console
.venv/bin/pip install -e '.[test]'
.venv/bin/python -m pytest -q
```

## Initialize a vault

Choose an empty path. Initialization refuses to overwrite a non-empty target.

```console
corum init /path/to/private-vault
cd /path/to/private-vault
corum doctor
```

The vault receives `corum.yaml`, `AGENTS.md`, `skills/`, `templates/`, and an empty
`courses/` directory. These agent assets are included in the installed wheel; vault
initialization does not need the source checkout.

## Environment variables

Secrets enter only through the process environment and are never accepted in YAML:

| Variable | Needed when |
| --- | --- |
| `CORUM_CANVAS_TOKEN` | Performing Canvas capture |
| `CORUM_JIRA_EMAIL` | Applying an enabled Jira plan |
| `CORUM_JIRA_API_TOKEN` | Applying an enabled Jira plan |

Disabled Jira performs no Jira credential check or client construction. Wiki
authoring uses local files and the active agent environment; no wiki credential is
defined by Corum v1.

## Workspace configuration

Edit `corum.yaml` with vault defaults. Example values are illustrative:

```yaml
schema: 1
workspace:
  timezone: Region/City
  term: Academic term
canvas:
  host: https://canvas.example.invalid
features:
  jira:
    enabled: true
  wiki:
    enabled: true
jira:
  site: https://jira.example.invalid
  project: STUDY
  transitions:
    this_week: "2"
calendar:
  timetable: Timetable.md
  term: Term_Calendar.md
```

Jira is enabled by default. If disabled for the whole vault, the workspace `jira`
mapping may be omitted. `workspace.timezone` must be an IANA zone name and controls
Canvas date conversion and run IDs. Canvas and Jira hosts must be credential-free
HTTPS origins (no path, query, fragment, or embedded user information).

## Course configuration

Create `courses/COURSE/course.yaml` with static course identity and watched Canvas
sources:

```yaml
schema: 1
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

When Jira is enabled, the course epic is required. When wiki is disabled, the wiki
mapping may be omitted.

Override either feature per course:

```yaml
features:
  jira:
    enabled: false
  wiki:
    enabled: true
```

Effective values resolve from built-in defaults, then workspace defaults, then
course overrides. Every run manifest records the result. A disabled feature assumes
none of its files, configuration, credentials, or network dependencies exist.

## Mechanical Canvas sync

Run deterministic capture from the vault root:

```console
corum sync COURSE --json
corum sync COURSE_A COURSE_B --dry-run --json
corum sync --all
```

Capture writes successful sources below `courses/COURSE/raw/`, advances only their
entries in `state/canvas.json`, and writes `state/latest-run.json`; a missing Canvas
state file is bootstrapped from the selected course's watched sources. Manifests
include stable item IDs, exact raw paths, structured details, per-source status, and
independent downstream stage results. A failed source remains retryable and is
reported as unknown. This command does not invoke an LLM, write Jira, or author wiki
prose.

## Agent-facing sync

Ask an agent that has opened the initialized vault to “sync COURSE.” `AGENTS.md`
routes the request to `skills/sync-course/SKILL.md`. That workflow:

1. runs mechanical capture and reads effective feature flags;
2. scopes only enabled Jira/wiki outputs from local state;
3. presents one combined approval question;
4. applies the exact approved Jira plan and authors approved wiki pages;
5. validates pages, index, provenance, and lint before finalizing sources; and
6. reports completed, failed, disabled, and retryable work separately.

The LLM is the only wiki author. Deterministic validators report objective defects
but do not generate replacement prose. The packaged linter's `--finalize` mode
validates an exact schema-1 payload and lint results before atomically updating wiki
state and the current run stage. Agents never edit those machine-owned files
directly. `state/wiki.json` stores only finalized source-to-provenance mappings,
never page IDs or content hashes.

## Exact Jira application

`corum jira apply` accepts a strict JSON plan on standard input:

```console
corum jira apply COURSE --dry-run < plan.json
corum jira apply COURSE < approved-plan.json
```

Create, update, and transition actions are a closed union. The command validates the
whole plan and proves every update/transition target belongs to the configured epic
before the first write. It applies sequentially, atomically updates the normalized
Jira cache after each successful action, and returns structured applied/failure and
retry evidence. An uncertain or partial write blocks nonempty plans until an exact
empty-plan reconciliation succeeds; fresh scoping and approval against the refreshed
cache are then required so the old plan cannot be duplicated automatically.

## No protocol-server dependency

Corum uses first-party HTTP clients for Canvas and Jira. It installs and runs without
any Model Context Protocol server, project server configuration, or external server
submodule. Normal tests use controlled transports and require no live credentials.

## Scope

Corum v1 has no hosted service, scheduler, database, LMS other than Canvas, tracker
other than Jira, automatic Git operation, or migration command. Existing vaults
require a separately reviewed manual migration.

See [SECURITY.md](SECURITY.md) for secret handling and vulnerability reporting.
