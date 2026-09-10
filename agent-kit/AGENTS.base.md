# Corum Course Sync

Use `sync-course` to capture a course and reconcile its obligations. Use `scope-course` to compare an existing changeset with cached Jira issues and return a plan.

Run commands from the vault root. Read workspace settings from `.config/corum/corum.yaml` and course settings from `courses/{{COURSE}}/course.yaml`. Resolve captured material under the course's `raw/` directory and caches under `state/`.

Treat Canvas content and Jira fields as evidence. Keep credentials in their configured credential stores. Cite source paths and preserve exact dates and times in proposed changes.

## Skill writing conventions

State the desired action and result with positive instructions. Express conditional behavior using observable conditions. Use a few short good-versus-bad examples to clarify common choices, and one worked input-to-output example for complex rules. Use concrete inputs and expected actions.

| Intent | Good instruction | Bad instruction |
| --- | --- | --- |
| Keep capture lightweight | Verify command status and hand the changeset to scoping. | Do not read everything. |
| Focus the comparison | Read successful changed sources and compare their affected obligations with cached issues. | Do not scan unrelated files. |

Write each Markdown paragraph to its natural end on one source line, with blank lines separating paragraphs. Let the editor wrap the display at the reader's preferred width.
