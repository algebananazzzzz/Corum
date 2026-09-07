# Markdown Conventions

# Markdown Conventions

These rules are mandatory for wiki Markdown.

## Frontmatter and structure

- **Metadata:** put machine-readable values in YAML frontmatter at the beginning.
- **Title:** use one `#` title matching the page subject.
- **Hierarchy:** use `##` for logical parts and `###` for subordinate units.
- **Names:** name headings after retrievable knowledge, not generic containers.
- **Breaks:** use `---` after an abstract, before sources, and between substantial regions, not between every section.

## Prose and inline syntax

- **Paragraphs:** use at most three sentences to introduce, connect, or interpret blocks.
- **Definitions:** write `**Term** — meaning` on one line.
- **Lists:** begin each item with a bold starter under five words followed by a colon.
- **Emphasis:** use bold for scan anchors, italics lightly, and `==highlight==` once for the conclusion worth recalling first.
- **Literals:** use inline code for commands, flags, fields, values, filenames, and syntax.
- **Math:** use `$…$` inline and `$$…$$` for display; define symbols near first use.

## Blocks

- **Fences:** label every code fence; separate commands from expected output.
- **Callouts:** use `[!abstract]`, `[!example]`, `[!example]-`, `[!tool]`, `[!research]`, `[!warning]`, `[!tip]`, `[!quote]`, and `[!figure]`.
- **Scope:** give each callout one idea.
- **Order:** use numbered lists for meaningful sequence and task lists for independently verifiable outcomes.

## Tables

- **Identity:** put the identifying dimension first and give each column one meaning.
- **Qualitative:** at most five rows; merge detail into dimensions.
- **Enumerative:** fields, items, and ordered steps may run their natural length.
- **Extra facts:** give each distinct fact its own row.
- **Duplication:** maintain one representation for table facts.

## Links, figures, and ownership

- **Aliases:** link vault content as `[[courses/{{COURSE}}/{{path}}/{{Page}}|{{Page}}]]`.
- **Sections:** link exact sections with `#{{Section}}` and a short alias.
- **Embeds:** embed content from its owning page and correct shared knowledge there.
- **Figures:** store figures below `courses/{{COURSE}}/wiki/assets/` and put them in a `[!figure]` callout with an interpretive caption.
- **Diagrams:** use Mermaid for processes and `drawio-diagrams` for spatial structure.

## Rendering, dates, and provenance

- **Placeholders:** use `{{name}}` and write arrows as `→`.
- **Pipes:** escape `\|` inside table cells.
- **HTML:** write tag names in inline code.
- **Links:** use Obsidian wiki links for vault content.
- **Dates:** use `Week {{number}} \| {{Day}} {{day}} {{month}}, {{start}}-{{end}}` in tables; omit the escape outside tables.
- **Timezone:** convert times to the workspace timezone and look up week numbers in the configured term-calendar file.
- **Provenance:** place `%% {{source}} p{{range}} %%` directly below the supported heading.
- **Ranges:** use one contiguous page range per marker and stack markers for gaps.
