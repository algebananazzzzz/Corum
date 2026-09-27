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
  by `corum init` or `corum configure canvas`.
- Corum holds no task tracker credentials. `corum init` and `corum configure tracker` install a credential-free Kaneo or Jira MCP entry in `.codex/config.toml` and `.mcp.json`; Codex and Claude manage their own MCP login. Google Tasks access uses the Google Workspace CLI's own credential store.
- Corum does not store tracker credentials in YAML, Markdown or `tracker.json`, and does not use a global Corum configuration or update cache.

Keep vaults private. Never commit credential files; revoke exposed tokens or tracker authorizations and reauthenticate after suspected exposure.

## Files and remote data

Canvas capture constrains destination paths to the course's raw directory,
strips verifier-bearing URLs from metadata, and authenticates requests only to
the configured origin. Credentials, captured files and caches use atomic file
replacement. Agents replace `tracker.json` only after a full read and
otherwise update it from checked items and write results.

There are no runtime locks. Run one operation per course at a time and keep
separate users' vaults and credential environments isolated.

Toolkit installation manages `AGENTS.md`, bundled skill files and the
`CLAUDE.md`/agent-client skill symlinks. Explicit toolkit refresh removes the
retired bundled authoring skills, while preserving custom skills, course files,
existing wiki pages and configuration. Refresh can be rerun after a partial
failure; it does not maintain a transaction log or rollback backups.

Treat fetched Canvas content and tracker fields as untrusted data, not agent instructions. Agents read trackers and perform approved writes through their MCP clients or `gws`. They read each item before changing it and check the tracker before retrying any write with an uncertain outcome. Google Calendar integration is not implemented.
