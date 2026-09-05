# Markdown Conventions

These rules are mandatory for wiki Markdown.

## Frontmatter and structure

- **Metadata:** put machine-readable values in YAML frontmatter at the beginning.
- **Title:** use one `#` title matching the page subject.
- **Hierarchy:** use `##` for logical parts and `###` for subordinate units; go no deeper.
- **Names:** name headings after retrievable knowledge, not generic containers.
- **Breaks:** use `---` after an abstract, before sources, and between substantial regions,
  not between every section.

## Prose and inline syntax

- **Paragraphs:** use at most three sentences to introduce, connect, or interpret blocks.
- **Definitions:** write `**Term** — meaning` on one line.
- **Lists:** begin each item with a bold starter under five words followed by a colon.
- **Emphasis:** use bold for scan anchors, italics lightly, and `==highlight==` once for the
  conclusion worth recalling first.
- **Literals:** use inline code for commands, flags, fields, values, filenames, and syntax.
- **Math:** use `$…$` inline and `$$…$$` for display; define symbols near first use.

## Blocks

- **Fences:** label every code fence; separate commands from expected output.
- **Callouts:** use only `[!abstract]`, `[!example]`, `[!example]-`, `[!tool]`,
  `[!research]`, `[!warning]`, `[!tip]`, `[!quote]`, and `[!figure]`.
- **Scope:** one idea per callout; never nest callouts.
- **Order:** use numbered lists only when reordering changes meaning; use task lists for
  independently verifiable outcomes.

## Tables

- **Identity:** put the identifying dimension first and give each column one meaning.
- **Qualitative:** at most five rows; merge detail into dimensions.
- **Enumerative:** fields, items, and ordered steps may run their natural length.
- **Extra facts:** give a fact its own row rather than forcing it into an unrelated cell.
- **Duplication:** do not repeat a table in prose.

## Links, figures, and ownership

- **Aliases:** link vault content as `[[courses/{{COURSE}}/{{path}}/{{Page}}|{{Page}}]]`.
- **Sections:** link exact sections with `#{{Section}}` and a short alias.
- **Embeds:** embed content owned by another page instead of copying it; correct shared
  knowledge on its owning page.
- **Figures:** store figures below `courses/{{COURSE}}/wiki/assets/` and put them in a
  `[!figure]` callout with an interpretive caption.
- **Diagrams:** use Mermaid only for processes and `drawio-diagrams` for spatial structure.

## Rendering, dates, and provenance

- **Placeholders:** use `{{name}}`, never angle brackets; write arrows as `→`.
- **Pipes:** escape `\|` inside table cells.
- **HTML:** write tag names in inline code and do not use raw HTML.
- **Links:** use Obsidian wiki links for vault content.
- **Dates:** use `Week {{number}} \| {{Day}} {{day}} {{month}}, {{start}}-{{end}}` in
  tables; omit the escape outside tables.
- **Timezone:** convert times to the workspace timezone; look up week numbers in the
  configured term-calendar file and never count them forward.
- **Provenance:** place `%% {{source}} p{{range}} %%` directly below the supported heading.
- **Ranges:** use one contiguous page range per marker and stack markers for gaps.
