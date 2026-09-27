# Jira

| Convention | Jira |
| --- | --- |
| Access | The `atlassian-jira` MCP server that Corum installs; available when the client lists its tools. Pass the saved `jira.site` URL as `cloudId` to every call |
| Project | `jira.project`, the project key; `corum init` saves `jira.site` |
| Required issue types | Issue type `Epic`; `Session` and `Milestone` are used when present. Read them with `getJiraProjectIssueTypesMetadata` |
| Course group | The epic in `jira.project` whose summary is exactly `{{COURSE}}`: `{"id": "<epic key>", "name": "{{COURSE}}"}` |
| Belongs to the course | Children of the epic |
| Misnamed | An epic whose summary starts with the code, such as `CS3103 Computer Networks` |

```yaml
jira:
  site: https://your-team.atlassian.net
  project: STUDY
```

## Operations

| Operation | Call |
| --- | --- |
| List projects | `getVisibleJiraProjects` for the site URL |
| Find course group | `searchJiraIssuesUsingJql` with `project = <jira.project> AND issuetype = Epic AND summary ~ "{{COURSE}}"`, keeping the exact summary |
| Create course group | `createJiraIssue` with issue type `Epic` and summary `{{COURSE}}` |
| Rename course group | `editJiraIssue` with summary `{{COURSE}}` |
| List items | Count once with `searchJiraIssuesUsingJql`, `jql: parent = <epic key>` and `searchResultMode: count`. Then fetch with `jql: parent = <epic key> ORDER BY key ASC`, `maxResults: 100`, `responseContentFormat: markdown` and `fields: ["summary", "status", "issuetype", "duedate", "labels", "description", "updated"]`, following `nextPageToken` until none is returned |
| Full read check | The fetched issue count equals the count |
| Get item | `getJiraIssue` with the issue key, `responseContentFormat: markdown` and the **List items** fields; not found means deleted |
| Find item by title | `searchJiraIssuesUsingJql` with `parent = <epic key> AND summary ~ "<title>"`, keeping the exact summary |
| Create item | `createJiraIssue` in `jira.project` with the issue type, summary, Markdown description, parent epic key, due date and labels |
| Update item | `editJiraIssue` with the changed fields |
| Move item | `getTransitionsForJiraIssue`, then `transitionJiraIssue` with the chosen transition ID |

Parameter names for parent, due date and labels differ between tool versions.

## tracker.json fields

| Field | Jira value |
| --- | --- |
| `id` | Issue key |
| `title` | Summary |
| `status` | Status name |
| `done` | The status category key is `done` |
| `due` | `duedate` |
| `type` | The lowercased issue type when it is `Task`, `Session` or `Milestone`, otherwise a `task`, `session` or `milestone` label |
| `labels` | The remaining labels |
| `description` | The Markdown body |
| `url` | `<site>/browse/<key>` |
| `updated_at` | `updated`, with a colon in its offset |

## Item format

| Field | Jira value |
| --- | --- |
| Title | Summary `[{{COURSE}}] Name` |
| Course | Parent epic |
| Type | Issue type `Task`, `Session` or `Milestone`; when the project lacks the type, use `Task` plus a `session` or `milestone` label |
| Categories | Labels, such as `lab` or `must-attend` |
| Due, task | Date only. For a true local deadline before 1500, use the preceding date so a workday remains; keep the true deadline in the description |
| Due, session or milestone | The date it occurs |
| To-do status | Any status in the `To Do` category |
| This Week status | A status named `This Week` reachable by a transition; none when the workflow lacks one |
