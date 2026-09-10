# Authoring Markdown: First Transfer Test

The two smaller models reproduced the core style on a new subject. Each first draft needed one editorial correction; both addressed the finding in a guided revision. These results support the reference as a working draft, with the user's taste still the final acceptance standard.

## Trial

- **Models:** GPT-5.6 Luna and GPT-5.6 Sol, both at medium reasoning effort.
- **Task:** the same [DHCP exercise](brief.md), using supplied facts and the authoring reference.
- **Context:** separate agent sessions received the task without the conversation history. The brief disclosed the evaluation criteria.
- **Preservation:** original drafts remain alongside their revisions.

| Model | First draft finding | Revision |
| --- | --- | --- |
| [Luna](dhcp-luna.md) | Diagram merged relay and local-network roles | [Uses a local broadcast domain](dhcp-luna-revised.md) |
| [Sol](dhcp-sol.md) | Example repeated all five values from its table | [Applies the table's configuration](dhcp-sol-revised.md) |

## Reference Improvements

- **Participant roles:** each diagram participant has one concrete role.
- **Fact ownership:** examples apply a table's facts and refer to their existing home.
- **Legacy conflict:** the old conventions file now redirects to the unified reference. Luna encountered its former em-dash convention during the initial trial.

## Verification

- **Mechanical checks:** approved ARP sample, reference, and revised drafts each have one title, labeled and balanced code fences, clean line endings, and zero em dashes.
- **Instruction wording:** the reference uses positive conventions; a scan for common negative imperatives returned zero matches.
- **Editorial review:** both revised drafts start with the defining exchange, use colon definitions and short list labels, and preserve the exercise's offer/acknowledgement, conflict-check, and delivery qualifications.
- **Scope:** this tests one new networking subject and guided correction. Draw.io generation, research quality, other domains, and rendered Mermaid appearance remain outside this trial.

The approved style example is [ARP](../../agent-kit/skills/authoring-wiki/examples/arp.md). The consolidated guidance is [Authoring Markdown](../../agent-kit/skills/authoring-wiki/references/authoring-markdown.md).

## Transfer Beyond Networking

After the reference gained examples from several subjects, GPT-5.6 Luna at medium reasoning effort received a fresh conditional-probability task. The supplied facts covered the conditional-probability formula and a school of 200 students: 80 club members, 50 music students, and 20 in both groups.

- **First draft:** [Conditional probability](conditional-probability-luna.md) used a formula-led explanation and a concrete worked example, with no networking structure carried over.
- **Finding:** inline LaTeX appeared inside plain parentheses, despite the guide specifying dollar delimiters.
- **Reference change:** added an explicit Bad/Good pair for inline math delimiters.
- **Guided revision:** [Corrected draft](conditional-probability-luna-revised.md) wraps inline expressions in dollar delimiters. Diff review confirmed that the original prose remained intact.
- **Limit:** this demonstrates transfer to one additional subject and successful guided correction. It is not evidence of flawless first-pass authoring or rendered-math verification.
