# Stored state

| File | Stored fields |
| --- | --- |
| `.config/corum/corum.yaml` | Selected timezone and academic term; Canvas URL; selected task tracker with Jira site/project or Kaneo URL/project |
| `courses/<course>/course.yaml` | Course code, which also names the course in the tracker; Canvas ID, source selection, optional name/folder mappings |
| `state/canvas.json` | Per-source comparison ledgers and syllabus hash |
| `state/tracker.json` | Agent-written copy of the course's tracker items in one shape for every tracker: tracker, course group, last sync time, and items with ID, title, type, status, completion, due date, category labels, description, URL and update time |

SGT (`Asia/Singapore`) is the setup default. The selected academic term resolves
its bundled calendar internally and installs `Term_Calendar.md` for week lookup.
There are no configurable calendar paths or Jira transition mappings; agents query available statuses and transitions through the tracker.

Stored records have no format-version field. Formats are unstable: there are no
migration adapters or backwards-compatibility promises. Optional values are
omitted when absent. No run history or workflow-stage records are stored.

`go test ./schemas` checks the schemas against actual serializers and useful valid/invalid shapes. `tracker-state.schema.json` has no Go serializer; its tests use example `tracker.json` files. Runtime validation also checks IANA timezones, parsed URLs and filesystem containment.
