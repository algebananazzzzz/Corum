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

`corum init` is an interactive terminal wizard. For automation or a minimal
Canvas-first vault, use `corum init --defaults /path/to/private-vault`; defaults
never open a browser and leave Jira disabled.

The vault receives `corum.yaml`, `AGENTS.md`, `skills/`, `templates/`, and an empty
`courses/` directory. These agent assets are included in the installed wheel; vault
initialization does not need the source checkout.

## Authentication

Canvas keeps its existing environment-variable authentication:

| Variable | Needed when |
| --- | --- |
| `CORUM_CANVAS_TOKEN` | Performing Canvas capture |

Jira uses Atlassian browser OAuth only—there is no Jira email or API-token setup.
Jira Cloud's Free plan is sufficient for a small personal or friends workspace;
Atlassian currently documents a limit of 10 users. Run:

```console
corum jira login /path/to/private-vault
corum jira status /path/to/private-vault
corum jira logout
```

Login opens Atlassian in the default browser and lets you select an accessible Jira
project. OAuth material is stored outside the vault in the platform user
configuration directory (`~/.config/corum/auth.json` on Linux unless
`XDG_CONFIG_HOME` is set). On POSIX systems Corum enforces directory mode `0700` and
file mode `0600`. Do not copy or commit that cache. Disabled Jira and Jira dry-runs
do not open a session or browser.

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
  cloud_id: 01234567-89ab-cdef-0123-456789abcdef
  site: https://jira.example.invalid
  project: STUDY
  transitions:
    this_week: "2"
calendar:
  timetable: Timetable.md
  term: Term_Calendar.md
```

Jira is disabled in newly generated defaults. Browser login enables it and writes
the selected non-secret `cloud_id` and project. `site` is optional display metadata
because Rovo may expose only the stable cloud ID. Existing version-1 vaults without
`cloud_id` still load, but must run `corum jira login` before a real Jira operation.
`workspace.timezone` must be an IANA zone name and controls Canvas date conversion
and run IDs. Canvas and optional Jira hosts must be credential-free HTTPS origins
(no path, query, fragment, or embedded user information).

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

## Local-first Jira connection

Canvas uses Corum's first-party HTTP client. Jira operations use Atlassian's hosted
Rovo MCP v2 endpoint so the official MCP SDK can perform browser OAuth, refresh the
session, and invoke Jira tools. Corum does not require the Rovo CLI, a local protocol
server, project server configuration, keyring, or an external server submodule.
Normal tests use controlled doubles and require no live credentials.

## Scope

Corum v1 has no hosted service, scheduler, database, LMS other than Canvas, tracker
other than Jira, automatic Git operation, sandbox, container orchestration, or
multi-user account service. Each person runs the CLI under their own operating-system
account and authorizes their own Atlassian account. Sandboxing is deferred until a
future hosted or untrusted-workload use case requires it.

See [SECURITY.md](SECURITY.md) for secret handling and vulnerability reporting.
