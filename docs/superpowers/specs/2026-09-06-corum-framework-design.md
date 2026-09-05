# Corum Framework Design

## Purpose

Corum is a content-free, agent-assisted framework for maintaining private academic workspaces. It captures course material from Canvas deterministically, then lets an LLM optionally synchronize Jira and author an Obsidian wiki under explicit skill instructions.

The public Corum repository contains software, schemas, agent instructions, templates, and tests. It never contains a user's course material, Canvas identifiers, Jira data, timetable, generated wiki, or credentials.

## Product boundary

Corum has two distinct artifacts:

1. **Framework repository** — public, versioned, and reusable.
2. **User vault** — private content and state belonging to one user.

The framework is installed into a Python environment. It is not copied into each vault and does not discover vaults relative to its own installation directory. Every command resolves an explicit vault path or uses the current working directory.

Corum does not host data, manage multiple tenants, or expose a network service. A maintainer may run the same installed framework against two or three isolated vaults, but each invocation receives only one vault and that user's credentials.

## Goals

- Provide deterministic, idempotent Canvas capture without Canvas MCP.
- Keep Jira synchronization optional but enabled by default.
- Keep wiki generation optional and LLM-authored.
- Provide one agent-facing sync workflow for capture, scoping, approval, application, validation, and finalization.
- Keep semantic decisions in skills and deterministic invariants in Python.
- Preserve independent state for Canvas, Jira, and wiki stages.
- Allow a vault to disable Jira or wiki globally or per course.
- Produce machine-readable run manifests so partial failure and retry state are observable.

## Non-goals

- No Canvas, Jira, or other MCP server integration.
- No hosted SaaS, web application, database, scheduler, or multi-tenant control plane.
- No Python code that decides what concepts mean or writes educational prose.
- No `corum migrate` command. Existing AcademicsWiki content will be migrated manually after Corum's v1 format is implemented.
- No automatic Git commits or remote repository management.
- No alternative LMS provider in v1.
- No tracker provider other than Jira in v1.

## Repository structure

```text
Corum/
├── src/
│   └── corum/
│       ├── __init__.py
│       ├── cli/
│       │   ├── main.py
│       │   ├── init.py
│       │   ├── sync.py
│       │   └── doctor.py
│       ├── config/
│       │   ├── models.py
│       │   ├── loader.py
│       │   └── secrets.py
│       ├── workspace/
│       │   ├── paths.py
│       │   ├── discovery.py
│       │   ├── locking.py
│       │   └── state.py
│       ├── canvas/
│       │   ├── client.py
│       │   ├── capture.py
│       │   ├── convert.py
│       │   ├── placement.py
│       │   └── models.py
│       ├── jira/
│       │   ├── client.py
│       │   ├── cache.py
│       │   └── apply.py
│       ├── validation/
│       │   ├── markdown.py
│       │   ├── links.py
│       │   └── provenance.py
│       └── runs/
│           ├── manifest.py
│           └── reporting.py
├── agent-kit/
│   ├── AGENTS.base.md
│   ├── skills/
│   │   ├── sync-course/
│   │   ├── scope-jira/
│   │   ├── scope-wiki/
│   │   ├── author-wiki/
│   │   ├── lint-wiki/
│   │   └── drawio-diagrams/
│   └── templates/
│       ├── workspace/
│       ├── course/
│       └── wiki/
├── schemas/
│   ├── corum.schema.json
│   ├── course.schema.json
│   ├── canvas-state.schema.json
│   ├── jira-state.schema.json
│   ├── wiki-state.schema.json
│   └── run-manifest.schema.json
├── tests/
│   ├── unit/
│   ├── integration/
│   └── fixtures/
├── pyproject.toml
├── README.md
├── LICENSE
└── SECURITY.md
```

Only first-party, importable Python code belongs under `src/corum`. Skills and templates remain visible under `agent-kit` because they are instructions and source assets rather than Python modules.

## Responsibility boundaries

### Python

Python owns work whose correctness can be expressed deterministically:

- configuration loading and validation;
- secret resolution;
- workspace discovery and path containment;
- per-course locking;
- Canvas HTTP requests, pagination, throttling, downloads, and conversion;
- upstream change detection;
- atomic state writes;
- Jira REST calls using an exact approved plan;
- Jira cache normalization;
- Markdown, link, index, and provenance validation;
- structured run manifests and terminal reports.

Python must not decide which concept page a lecture belongs to, what coverage is educationally important, or what prose belongs in a wiki page.

### LLM skills

Skills own semantic judgment and orchestration:

- interpreting captured changes;
- scoping Jira creates and updates;
- scoping concept creation and enrichment;
- authoring or revising wiki pages;
- deciding source coverage and deliberate omissions;
- presenting one approval gate before Jira or wiki mutation;
- coordinating deterministic helpers and reporting their results.

The Jira scoper and wiki scoper are independent. Neither feature is allowed to assume the other is enabled.

## User vault format

```text
vault/
├── corum.yaml
├── AGENTS.md
├── Timetable.md
├── Term_Calendar.md
├── courses/
│   └── CS3103/
│       ├── course.yaml
│       ├── Conventions and Milestones.md
│       ├── Changelog.md
│       ├── state/
│       │   ├── canvas.json
│       │   ├── jira.json
│       │   ├── wiki.json
│       │   └── latest-run.json
│       ├── raw/
│       └── wiki/
│           ├── index.md
│           ├── explainers/
│           ├── concepts/
│           ├── references/
│           └── assets/
└── .obsidian/
```

`course.yaml` is human-owned configuration. Files under `state/` are machine-owned. The `jira.json` file is absent when Jira has never been enabled for the course. The `wiki.json` file and `wiki/` directory are absent when wiki generation has never been enabled.

## Workspace configuration

`corum.yaml` defines vault-wide defaults:

```yaml
schema: 1

workspace:
  timezone: Asia/Singapore
  term: AY2026/27 Semester 1

canvas:
  host: https://canvas.example.edu

features:
  jira:
    enabled: true
  wiki:
    enabled: true

jira:
  site: https://example.atlassian.net
  project: STUDY
  transitions:
    this_week: "2"

calendar:
  timetable: Timetable.md
  term: Term_Calendar.md
```

Canvas capture is mandatory and has no feature switch. Jira is optional and enabled by default. Wiki generation is optional and enabled by default. A disabled Jira feature permits the entire `jira` section to be absent. An enabled Jira feature requires `site` and `project`.

Secrets are never accepted in YAML. Corum v1 reads `CORUM_CANVAS_TOKEN`, `CORUM_JIRA_EMAIL`, and `CORUM_JIRA_API_TOKEN` from the process environment. Jira secrets are required only when an enabled Jira stage is applied.

## Course configuration

`course.yaml` defines static course identity and optional overrides:

```yaml
schema: 1
code: CS3103

canvas:
  id: 93794
  sources:
    - announcements
    - assignments
    - files
    - pages
    - modules
    - syllabus
  folders:
    Lecture Notes & Readings: lectures
    Labs: labs

jira:
  epic: STUDY-1

wiki:
  split_rules: default
```

A course overrides workspace feature defaults only when it declares an override:

```yaml
features:
  jira:
    enabled: false
  wiki:
    enabled: true
```

Resolution order is built-in default, then `corum.yaml`, then `course.yaml`. The effective configuration is recorded in every run manifest.

When Jira is effectively enabled, `jira.epic` is required. When wiki is effectively disabled, `wiki` configuration may be absent.

## State ownership

### `state/canvas.json`

Canvas owns its capture ledger and last successful capture time:

```json
{
  "schema": 1,
  "synced_at": "2026-09-06T12:00:00+08:00",
  "sources": {
    "announcements": {},
    "assignments": {},
    "files": {},
    "pages": {},
    "modules": {},
    "syllabus": null
  }
}
```

An absent source key means the course does not watch that source. An empty object means the source is watched but nothing has been recorded. A source that fails to read does not lose or advance its prior state.

### `state/jira.json`

Jira owns only its normalized cache:

```json
{
  "schema": 1,
  "reconciled_at": null,
  "issues": []
}
```

The epic belongs in `course.yaml`, not Jira state. Successful Jira writes are followed by atomic cache upserts. A Jira write whose cache update fails is reported as partially applied and requires reconciliation before retry.

### `state/wiki.json`

Wiki state records source finalization independently of Canvas capture:

```json
{
  "schema": 1,
  "ingested": {}
}
```

A source is marked ingested only after every planned page, index edit, and validation required for that source succeeds.

### `state/latest-run.json`

The latest run manifest records stage status and supports explicit partial failure:

```json
{
  "schema": 1,
  "run_id": "20260906T120000+0800",
  "course": "CS3103",
  "effective_features": {
    "jira": true,
    "wiki": false
  },
  "canvas": {
    "status": "changed",
    "changes": [],
    "failures": []
  },
  "jira": {
    "status": "pending"
  },
  "wiki": {
    "status": "disabled"
  }
}
```

Allowed stage statuses are `disabled`, `pending`, `up_to_date`, `changed`, `applied`, `partial`, and `failed`. Disabled stages perform no reads, writes, validation, or credential checks for that feature.

## Command contract

Corum exposes four commands in v1:

```text
corum init
corum sync COURSE [COURSE ...] [--all] [--dry-run] [--json]
corum doctor
corum jira apply COURSE
```

`corum init` creates a new private vault skeleton and refuses to overwrite existing paths.

`corum sync` is the deterministic Canvas primitive. It validates configuration, locks each selected course, captures watched sources, writes `raw/`, advances only successfully read Canvas state, writes `latest-run.json`, and prints either human-readable or JSON output. It does not invoke an LLM, author wiki pages, or write Jira.

`corum doctor` validates configuration, schemas, required files, enabled-feature requirements, credentials, path containment, and Canvas/Jira connectivity without mutating the vault.

`corum jira apply` accepts an exact structured plan from standard input, validates that it targets the selected course and configured epic, applies changes sequentially, and atomically upserts the normalized cache after each successful write. It never invents or rescopes actions.

## Agent-facing one-step sync

The one-step experience belongs to the `sync-course` skill. A user asks the agent to sync a course; the skill performs:

```text
corum sync --json
        ↓
read effective features and captured change manifest
        ├── Jira enabled → scope Jira plan
        └── Wiki enabled → scope wiki plan
        ↓
present one combined approval question
        ↓
apply approved Jira plan and author approved wiki changes
        ↓
validate, finalize successful state, and update changelog
```

If Canvas reports no changes, the skill reports the result and stops. If a Canvas source fails, it is reported as unknown rather than clean. Independent Jira and wiki actions may continue when one action fails, but failed work never advances its feature state.

## Error and retry behavior

- Configuration and schema failures stop before network or content writes.
- Course locks prevent overlapping syncs against the same course.
- Canvas sources run independently; one unread source does not erase or advance another source's state.
- Raw writes are constrained beneath the selected course's `raw/` directory.
- State files are replaced atomically.
- Jira creates and updates are sequential so returned keys and cache changes remain unambiguous.
- Wiki authoring failures leave affected sources unfinalized.
- Every partial application is visible in `latest-run.json` and the human report.
- Re-running capture is idempotent according to Canvas IDs, timestamps, due dates, and content digests rather than the wall-clock time of the prior run.

## Security

- No credential, token, verifier-bearing URL, or user content is committed to Corum.
- Example configuration uses non-routable or clearly illustrative hosts and identifiers.
- Secrets enter only through the process environment in v1.
- Logs and JSON reports exclude credentials and strip Canvas download verifiers.
- Upstream names and paths are untrusted and cannot escape the invited `raw/` directory.
- Each hosted user is executed in a separate process with one vault path and one credential environment.

## Testing strategy

Unit tests cover configuration resolution, path containment, state serialization, change detection, Canvas conversion, Jira cache normalization, and report status derivation.

Integration tests use temporary vaults and mocked HTTP transports. They prove:

- a Canvas-only course works without Jira or wiki files;
- Jira is enabled by default and can be disabled globally or per course;
- wiki is enabled by default and can be disabled globally or per course;
- disabled features require no configuration or credentials;
- partial Canvas failure preserves prior state;
- repeated capture produces no duplicate files or false changes;
- Jira applies only an exact validated plan and updates its cache atomically;
- no write can escape the selected vault or course;
- JSON output conforms to the published run-manifest schema.

Live Canvas and Jira credentials are never required by the normal test suite.

## Acceptance criteria

Corum v1 is complete when:

1. A fresh environment can install the framework without Canvas MCP or Atlassian MCP.
2. `corum init` creates a valid empty vault.
3. `corum sync COURSE --json` captures Canvas sources into the v1 vault layout and emits a schema-valid manifest.
4. A course can independently enable or disable Jira and wiki behavior.
5. Jira is enabled by default, while a disabled Jira configuration performs no Jira credential check or network call.
6. Wiki prose and semantic planning exist only in agent skills, not Python.
7. The `sync-course` skill provides one approval gate and completes the enabled stages without assuming disabled-stage files exist.
8. The existing AcademicsWiki vault remains unchanged until a separately reviewed manual migration plan is approved.

## Implementation sequence

This architecture is implemented as separately reviewable slices:

1. Package scaffold, schemas, configuration, and vault initialization.
2. First-party Canvas REST client and deterministic capture.
3. Run manifests, reporting, locking, and validation.
4. Optional direct Jira application and cache management.
5. Independent Jira and wiki skills plus the top-level sync skill.
6. Manual migration of AcademicsWiki into a private Corum vault.

Each slice must be runnable and tested before the next begins. The old AcademicsWiki and AcademicsWikiFramework repositories remain available as rollback references until manual migration and verification are complete.
