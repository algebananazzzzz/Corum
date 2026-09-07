# Corum

Corum is an agentic academic framework for students. It connects Canvas, Jira, and a course wiki so an LLM can turn scattered course material into clear work you can act on and help you catch every deadline.

Course requirements rarely live in one place. An assignment can omit its due date while an announcement supplies it. A lecture slide can explain a requirement that the assignment page assumes. A tutorial can introduce a concept that makes the next lecture easier to understand. Corum captures this material into a private local vault, where an agent can connect the evidence, surface obligations, create Jira tasks, and build a useful wiki from your course content.

## What Corum does

- Captures Canvas announcements, assignments, files, pages, modules, and syllabus material into your local course vault.
- Gives your LLM course-aware skills for finding deadlines, requirements, milestones, and assessed work across those sources.
- Creates a reviewable Jira plan for tasks, sessions, and milestones when Jira is enabled for a course.
- Builds explainers, concept pages, references, diagrams, and a course guide from lecture slides and other course material when wiki authoring is enabled.
- Preserves source paths and page-range provenance so every wiki section can be traced back to course material.

## Get started

Install Corum, create a vault, and connect the services you want to use:

```console
curl -fsSL https://raw.githubusercontent.com/algebananazzzzz/Corum/main/install.sh | sh
corum init
cd path-to-vault
corum auth
corum doctor
```

`corum auth` connects Canvas and can connect Jira. Your vault contains the agent instructions, custom skills, course sources, wiki pages, and local workflow records.

Open an LLM coding agent in the vault directory after setup. Corum provides the skills and course context; you describe the outcome you want in plain language.

## Prompts for your LLM

Use prompts like these from inside your vault:

```text
Sync {{course}} and show me the Jira and wiki changes for approval.
```

```text
Review {{course}} for upcoming deadlines, including dates mentioned in announcements and lecture material.
```

```text
Catch up {{course}}. Create Jira tasks for outstanding required work and build wiki pages for the new lecture material.
```

```text
Build a beginner-friendly wiki explainer for the Week 4 transport-layer slides in {{course}}.
```

```text
Update the {{course}} course guide with the latest grading rules, weekly rhythm, milestones, and late policy.
```

```text
Audit the {{course}} wiki for missing source coverage, broken links, and concepts that need clearer explanations.
```

## How a course sync works

1. Corum captures the latest Canvas material for the selected course.
2. The agent reads the captured sources and identifies course obligations, deadlines, changes, and wiki knowledge.
3. The agent presents separate Jira Changes and Wiki Changes sections for your approval.
4. Approved Jira work becomes tasks, sessions, or milestones. Approved wiki work becomes source-backed course pages.
5. Corum records the completed work in your local vault so later syncs build on the current course state.

Jira and wiki workflows operate independently. Enable either workflow for the courses where it helps, and use a fresh sync after changing a course’s service configuration.

## The course wiki

The wiki is a learning resource. It turns course material into progressive explainers, focused concept pages, rapid-lookup references, diagrams, and a course guide. Each page links to related knowledge and records the source material that supports it.

The result is a study resource that grows with the course: a place to revisit difficult concepts, find details quickly before an assignment, and understand how each lecture connects to the next.

## Privacy and ownership

Each Corum vault is a private local project. Your course sources, credentials, Jira records, wiki pages, and workflow history live in that vault. See [SECURITY.md](SECURITY.md) for the trust boundaries and security model.

## Updates

Update Corum with:

```console
corum update
```
