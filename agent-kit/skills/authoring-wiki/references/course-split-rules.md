# Course Concept Boundaries

Read `courses/{{COURSE}}/course.yaml` and use `wiki.split_rules` when present.
Interpret the value as the course's explicit concept-boundary instruction. If it is
`default` or absent, keep concept pages coarse and split only when a page becomes
heavy: roughly more than 200 lines or six content `##` sections, excluding Sources.

Split by independent purpose or mechanism, never merely by lecture, week, file, or
slide-deck boundary. Named examples stay inside the owning concept unless they have
their own purpose, lifecycle, constraints, and failure modes.
