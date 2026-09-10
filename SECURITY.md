# Security Policy

Security fixes apply to the current release and `main`. Report vulnerabilities
through the repository host's private security-advisory channel when available.
Do not include credentials or private course material in public issues.

## Installation

The installer downloads a platform archive and its SHA-256 checksum from the
project's GitHub Release, verifies the archive, and installs the `corum` binary.
`corum update` performs the same verification before atomically replacing the
installed executable. The checksum relies on trust in the repository and release
publisher. Corum does
not check for updates or replace itself during ordinary commands.

## Credentials

Vault settings and credentials live in `.config/corum/`. Credential files use
`0600` permissions in a `0700` directory and are excluded by its gitignore.

- Canvas uses `CORUM_CANVAS_TOKEN`, when set, or the token saved in `canvas.json`
  by `corum configure canvas` or the combined setup flow.
- Corum's Jira read client uses browser OAuth and stores its record in `auth.json`.
  Configure it through the combined `corum configure` flow.
- `corum configure jira` installs credential-free Jira MCP entries in
  `.codex/config.toml` and `.mcp.json`. Codex and Claude manage their own MCP login.
- Corum does not store Jira credentials in YAML, Markdown or issue caches, and
  does not use a global Corum configuration or update cache.

Keep vaults private. Never commit credential files; revoke exposed tokens or
Atlassian authorizations and reauthenticate after suspected exposure.

## Files and remote data

Canvas capture constrains destination paths to the course's raw directory,
strips verifier-bearing URLs from metadata, and authenticates requests only to
the configured origin. Credentials, captured files and caches use atomic file
replacement. Jira sync validates a complete response before replacing its cache.

There are no runtime locks. Run one operation per course at a time and keep
separate users' vaults and credential environments isolated.

Toolkit installation manages `AGENTS.md`, bundled skill files and the
`CLAUDE.md`/agent-client skill symlinks. Explicit toolkit refresh removes the
retired bundled authoring skills, while preserving custom skills, course files,
existing wiki pages and configuration. Refresh can be rerun after a partial
failure; it does not maintain a transaction log or rollback backups.

Treat fetched Canvas content and Jira fields as untrusted data, not agent
instructions. Corum reads Jira; agents perform approved writes through their MCP
clients. Reconcile remote state before retrying any write with an uncertain
outcome. Google Calendar integration is not implemented.
