# tracker.json

`courses/{{COURSE}}/state/tracker.json` holds the course's tracker items in one shape for every tool, so planning compares against a single format. Each tracker reference maps its fields onto this shape.

```json
{
  "tracker": "kaneo",
  "group": {"id": "CS3103", "name": "CS3103"},
  "synced_at": "2026-09-27T10:00:00+08:00",
  "items": [
    {
      "id": "a85mlhg03aqyre583jotzb6o",
      "title": "[CS3103] Lab (W7)",
      "type": "session",
      "status": "this-week",
      "done": false,
      "due": "2026-10-02T10:00:00+08:00",
      "labels": ["lab", "must-attend"],
      "description": "Week 7 | Fri 2 Oct, 1000-1200, COM1-B102\n\n**Attendance:** mandatory",
      "updated_at": "2026-09-26T18:25:47.030Z"
    }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `tracker` | `jira`, `kaneo` or `google_tasks` |
| `group` | The course group: an epic (`id` is its key), a label (`id` and `name` are the code) or a task list (`id` is the list ID); `null` when it does not exist yet |
| `synced_at` | When the file last matched the tracker, after a full read or a round of writes, with offset |
| `id` | The tool's item ID, used as the target of updates |
| `type` | `task`, `session` or `milestone`; omit when the item has none |
| `status` | The tool's status or column, verbatim |
| `done` | Whether that status is final |
| `due` | The tool's due date: a date, or a date-time with offset where the tool stores times |
| `labels` | Category labels only; the course and type are already in `group` and `type` |
| `description` | Item body as Markdown, without an appended labels line |
| `url`, `updated_at` | Include when the tool provides them |

Items are sorted by `id`, with two-space JSON indentation. Absent optional fields are omitted instead of written as `null`.
