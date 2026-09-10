# Stored state

| File | Stored fields |
| --- | --- |
| `.config/corum/corum.yaml` | Selected timezone and academic term; configured Canvas URL and Jira cloud/project |
| `courses/<course>/course.yaml` | Course code; Canvas ID, source selection, optional name/folder mappings; optional Jira epic |
| `state/canvas.json` | Per-source comparison ledgers and syllabus hash |
| `state/jira.json` | Current issue snapshot: key, type, summary, status; available due date, labels, description and update timestamp; snapshot freshness timestamp |

SGT (`Asia/Singapore`) is the setup default. The selected academic term resolves
its bundled calendar internally and installs `Term_Calendar.md` for week lookup.
There are no configurable calendar paths or Jira transition mappings; agents
query available transitions through Jira MCP.

Stored records have no format-version field. Formats are unstable: there are no
migration adapters or backwards-compatibility promises. Optional values are
omitted when absent. No run history or workflow-stage records are stored.

`go test ./schemas` checks the schemas against actual serializers and useful
valid/invalid shapes. Runtime validation also checks IANA timezones, parsed URLs
and filesystem containment.
