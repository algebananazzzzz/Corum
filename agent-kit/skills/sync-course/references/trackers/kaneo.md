# Kaneo

| Convention | Kaneo |
| --- | --- |
| Access | The `kaneo` MCP server that Corum installs for `kaneo.url`; available when the client lists its tools |
| Project | `kaneo.project`, the project slug, shared by all courses; `corum init` saves `kaneo.url` |
| Course group | The label `{{COURSE}}` on each of the course's tasks. Nothing needs to be created, so `group` in `tracker.json` is always `{"id": "{{COURSE}}", "name": "{{COURSE}}"}` |
| Belongs to the course | Tasks carrying the `{{COURSE}}` label or a title starting with `[{{COURSE}}]` |

```yaml
kaneo:
  url: https://kaneo.example.com
  project: TOD
```

## Operations

| Operation | Call |
| --- | --- |
| List projects | `list_workspaces`, then `list_projects` for each; show name, slug and `statistics.totalTasks` |
| Find project | The `list_projects` entry whose `slug` is `kaneo.project`, keeping its `id` and `workspaceId`, and its columns from `list_project_columns` |
| List items | `list_tasks` with `projectId`, `limit: 100` and `page` from 1 to `pagination.totalPages`. Tasks sit under `data.columns[].tasks`, `data.archivedTasks` and `data.plannedTasks`. A task's labels are complete only after also requesting `relatedPage` 2 through `pagination.relatedTotalPages` for that page |
| Full read check | The unique task IDs collected equal `pagination.total` |
| Get item | `get_task` with the task ID; not found means deleted. It returns no labels, so keep the labels from `tracker.json` |
| Find item by title | `search` with `type: tasks`, `projectId` and the title as `q`, keeping the exact title |
| Create item | `create_task` with `projectId`, `title`, `description`, `priority`, `dueDate` and `status` set to the to-do column's slug, then **Add label** for the course code, the type and each category |
| Add label | `create_label` with `name`, `workspaceId`, `taskId` and the label's color below |
| Remove label | `list_workspace_labels`, keeping the entry with that `name` and the task's `taskId`, then `detach_label_from_task` with its `id` |
| Update item | `update_task` with the changed fields |
| Move item | `update_task_status` with a column slug |

| Label | Color |
| --- | --- |
| Course code | `#64748b` |
| `task` | `#0ea5e9` |
| `session` | `#8b5cf6` |
| `milestone` | `#f43f5e` |
| Any category | `#94a3b8` |

## tracker.json fields

| Field | Kaneo value |
| --- | --- |
| `id` | Task ID |
| `title`, `description` | Verbatim |
| `status` | Column slug |
| `done` | The column has `isFinal: true` |
| `due` | `dueDate` converted to the workspace timezone, with its offset |
| `type` | The `task`, `session` or `milestone` label |
| `labels` | The remaining label names, without the course code |
| `url`, `updated_at` | Omitted; Kaneo returns neither |

## Item format

| Field | Kaneo value |
| --- | --- |
| Title | `[{{COURSE}}] Name` |
| Course | Label `{{COURSE}}` |
| Type | Label `task`, `session` or `milestone` |
| Categories | One label each, such as `lab` or `must-attend` |
| Due, task | The true local deadline as a date-time, for example `2026-10-02T12:00:00+08:00` |
| Due, session or milestone | Its local start time |
| Due, time unknown | `23:59` local on the known date |
| Priority | `high` for exams, `medium` otherwise |
| To-do status | The lowest-`position` column, such as `to-do` |
| This Week status | The column named `This Week` |

Kaneo stores `dueDate` in UTC; send it as UTC or with an explicit offset.
