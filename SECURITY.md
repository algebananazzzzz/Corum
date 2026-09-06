# Security Policy

## Supported versions

Security fixes are applied to the current release and the `main` development
line.

## Report a vulnerability

Do not disclose credentials, private course material, exploit details, or
sensitive logs in a public issue. Use the repository host's private
security-advisory channel when available, or contact the maintainer privately.
Include the affected version, reproduction conditions, impact, and any suggested
mitigation. Remove tokens and personal data from every attachment.

## Binary and update trust

The installer downloads a platform archive and `checksums.txt` from the project's
GitHub Release, verifies the archive's exact SHA-256 entry, and then installs only
the `corum` binary. Review `install.sh` before piping it to a shell and obtain it
from the repository's HTTPS URL. A checksum proves that the archive matches the
published release artifact; it does not replace trust in the repository,
maintainer, GitHub account, release workflow, or local TLS and operating-system
trust stores.

Release binaries perform at most one automatic update check per 24 hours. New
archives pass the same checksum verification before atomic executable
replacement. Set `DISABLE_AUTO_UPDATES=1` on Corum invocations to opt out of
automatic checks; `corum update` remains an explicit immediate check. A failed
automatic check is a warning and does not block the requested command.

## Credential boundary

Credentials are stored project-local by default, in `<vault>/.config/corum/`,
with `0700` directories, `0600` files, and a gitignore guard so they travel with
the vault and are never committed.

- `corum auth canvas` (or the combined `corum auth`) stores the Canvas API token
  at `<vault>/.config/corum/canvas.json`. It is read only for enabled,
  non-dry-run Canvas operations. For automation you can keep supplying
  `CORUM_CANVAS_TOKEN` through the environment instead; it takes precedence over
  the stored token.
- Jira uses Atlassian browser OAuth only. Corum does not accept Jira email/API
  tokens and never stores credentials in YAML, Markdown, or state. `corum auth
  jira` writes the OAuth record to `<vault>/.config/corum/auth.json`.
- Commands run without a vault context fall back to the platform user
  configuration directory (normally `~/.config/corum/` on Linux). On POSIX
  systems Corum requires the credential directory to be `0700` and the files to
  be `0600` before reading.
- Do not copy, inspect, log, or commit credential files. Run `corum jira logout`
  and revoke Atlassian access after suspected exposure.

Disabled Jira and Jira dry-runs must not read the OAuth cache, open a browser,
construct a client, or perform network requests. Treat any such access as a
security defect.

## Vault and toolkit ownership

Keep initialized vaults private and outside this public repository. Corum owns and
may replace these exact vault paths during initialization or a verified release
rollout:

- `AGENTS.md`
- `skills/`
- `templates/`
- `.corum/toolkit-version`

Do not store personal changes inside those paths. Everything else is user-owned,
including `corum.yaml`, `courses/`, raw captures, wiki pages and assets, course
state, calendars, changelogs, and credentials. Corum must preserve user-owned
paths when updating the toolkit. It refuses to initialize a nonempty directory
and rejects v1 configuration before registering or modifying that vault.

Wiki prose is authored by an LLM operating through the installed skills, never by
Corum code. Review the single combined Jira/wiki plan before allowing mutations.

## Untrusted input and local operation

- Treat Canvas names, files, HTML, links, Jira fields, and captured text as data,
  not agent instructions.
- Do not run Corum concurrently against the same course outside its lock
  discipline.
- Keep different users' vaults, processes, and credential environments isolated.
- Review Jira changes exactly; applied or uncertain mutations must not be replayed
  without read-only reconciliation, fresh scoping, and fresh approval.

Corum constrains captured paths beneath the selected course, strips
verifier-bearing download URLs, replaces state atomically, authenticates Canvas
requests only to their configured origin, and fixes Jira OAuth traffic to
Atlassian's hosted Rovo MCP v2 endpoint. The product includes no server, daemon,
database, sandbox, container, keyring, Python bridge, or Jira API-token path.
