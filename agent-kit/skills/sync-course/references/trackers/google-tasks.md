# Google Tasks

| Convention | Google Tasks |
| --- | --- |
| Access | The Google Workspace CLI, as `gws` or, when it is not installed, `npx --yes @googleworkspace/cli@0.22.5`. `gws auth status` reports Tasks access |
| Sign-in | The user runs `gws auth login -s tasks` in a terminal. The first login needs a Desktop OAuth client from Google Cloud Console, as the `gws` documentation describes |
| Project | None |
| Course group | The task list titled exactly `{{COURSE}}`: `{"id": "<list id>", "name": "{{COURSE}}"}` |
| Belongs to the course | Tasks in the list |
| Misnamed | A list whose title starts with the code, such as `CS3103 Computer Networks` |

## Operations

| Operation | Command |
| --- | --- |
| Find course group | `gws tasks tasklists list --params '{"maxResults":100}' --page-all`, keeping the exact `title` |
| Create course group | `gws tasks tasklists insert --json '{"title":"{{COURSE}}"}'` |
| Rename course group | `gws tasks tasklists patch --params '{"tasklist":"<id>"}' --json '{"title":"{{COURSE}}"}'` |
| List items | `gws tasks tasks list --params '{"tasklist":"<id>","showCompleted":true,"showHidden":true,"maxResults":100}' --page-all`, which prints one JSON page per line |
| Full read check | Every page has `"kind": "tasks#tasks"` and the last page has no `nextPageToken` |
| Get item | `gws tasks tasks get --params '{"tasklist":"<id>","task":"<task id>"}'`; not found or `"deleted": true` means deleted |
| Find item by title | **List items** with `"updatedMin"` set to `synced_at` from `tracker.json` and `"showDeleted":true` added, keeping the exact title |
| Create item | `gws tasks tasks insert --params '{"tasklist":"<id>"}' --json '{"title":...,"notes":...,"due":...}'` |
| Update item, Move item | `gws tasks tasks patch --params '{"tasklist":"<id>","task":"<task id>"}' --json '{...}'` |

## tracker.json fields

| Field | Google Tasks value |
| --- | --- |
| `id`, `title`, `status` | Verbatim |
| `done` | `status` is `completed` |
| `due` | The date part of `due` |
| `description` | `notes` without its final `Labels:` line |
| `type` | The first of `task`, `session` or `milestone` in the `Labels:` line |
| `labels` | The rest of the `Labels:` line |
| `url` | `webViewLink` |
| `updated_at` | `updated` |

## Item format

| Field | Google Tasks value |
| --- | --- |
| Title | `[{{COURSE}}] Name` |
| Course | The course's task list |
| Type and categories | A final notes line after a blank line, type first: `Labels: session, lab, must-attend` |
| Due | The true local deadline date as `YYYY-MM-DDT00:00:00.000Z`; Google Tasks keeps only the date, so the description carries the time |
| Done | `status: completed`; otherwise `needsAction` |
| To-do and This Week status | None; Google Tasks has no columns |
