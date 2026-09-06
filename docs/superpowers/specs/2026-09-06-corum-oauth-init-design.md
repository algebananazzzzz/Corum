# Corum OAuth Init Design

**Date:** 2026-09-06

## Goal

Give local Corum users a guided terminal setup experience and browser-based Jira
authentication without API tokens, keyrings, hosted Corum infrastructure, or
sandbox orchestration.

Corum remains a local-first CLI. Each user runs it on their own computer, owns
their own vault, and authorizes their own Atlassian account.

## Scope

This change will:

- make `corum init` an interactive, Huh-style terminal wizard;
- authenticate Jira through the Atlassian Rovo MCP v2 OAuth 2.1 flow;
- let the user choose an accessible Atlassian site and Jira project;
- persist the OAuth client registration and tokens in a portable per-user cache;
- route all Jira reads and writes through Rovo MCP tools;
- add explicit Jira login, logout, and status commands;
- keep Jira optional; and
- preserve the existing exact-plan approval boundary and recovery guarantees.

This change will not add a hosted service, database, keyring integration,
containers, cgroups, Docker, multi-tenant execution, or agent sandboxing.
Sandboxing is deferred until Corum runs hosted or untrusted workloads.

Canvas authentication is unchanged and continues to use
`CORUM_CANVAS_TOKEN`. “OAuth only” applies to Jira authentication.

## User Experience

### Interactive initialization

When stdin and stdout are interactive terminals, `corum init [path]` presents a
multi-step form:

1. Confirm the target vault path.
2. Enter the workspace timezone and academic term.
3. Enter the Canvas HTTPS origin.
4. Enable or disable wiki authoring.
5. Enable or disable Jira.
6. If Jira is enabled, connect an Atlassian account in the browser.
7. Choose one accessible Atlassian site.
8. Load projects with `listJiraProjects` and choose one Jira project from that
   site.
9. Review a redacted configuration summary and confirm creation.

The wizard validates each value before moving forward. Cancellation writes no
vault files. The existing refusal to initialize a non-empty target remains.

The terminal UI will use a Python-native prompt library rather than introduce a
Go helper binary. It should support keyboard navigation, plain validation
messages, Ctrl-C cancellation, and a non-ANSI fallback for limited terminals.

### Non-interactive initialization

`corum init --defaults [path]` preserves the current deterministic scaffold for
tests, automation, and terminals without an interactive input stream. It does
not attempt OAuth. Its generated workspace disables Jira because an enabled
OAuth-only Jira configuration cannot be completed non-interactively.

### Jira session commands

- `corum jira login [path]` opens the browser, completes OAuth, and replaces the
  current local Atlassian session. When `path` is an existing vault, it also
  asks the user to select a site and project, previews the non-secret YAML
  change, and updates `corum.yaml` only after confirmation.
- `corum jira status` checks the cached session and prints the authenticated
  account plus the current vault's selected site, without printing tokens.
- `corum jira logout` deletes the cached OAuth material. It does not alter any
  vault configuration.
- `corum jira apply COURSE` keeps its existing stdin plan and dry-run behavior.
  A real apply requires a valid OAuth session; it never falls back to an API
  token.

## Authentication Architecture

Corum connects to `https://mcp.atlassian.com/v2/mcp` with the official Python
MCP client SDK. The SDK performs protected-resource discovery, authorization
server discovery, dynamic client registration, PKCE generation, state and
issuer validation, authorization-code exchange, and refresh-token rotation.

For an interactive login, Corum:

1. binds a callback listener to an ephemeral port on `127.0.0.1`;
2. opens the SDK-produced Atlassian authorization URL in the default browser;
3. waits for the matching callback with a bounded timeout;
4. completes the token exchange;
5. calls `getAccessibleAtlassianResources`; and
6. returns the account and accessible sites to the wizard.

The callback listener accepts only the expected path and state. It shuts down
after success, cancellation, or timeout. Corum prints the authorization URL so
the user can open it manually when automatic browser launch fails.

## Portable Token Storage

No keyring is used. Corum implements the MCP SDK token-storage protocol with one
JSON document in the platform-standard per-user configuration directory,
resolved with `platformdirs`:

- Linux: `$XDG_CONFIG_HOME/corum/auth.json`, otherwise
  `~/.config/corum/auth.json`;
- macOS and Windows: the corresponding platform user configuration directory.

The document stores the OAuth client registration, access token, refresh token,
scope, and expiry required by the SDK. It contains no Jira or course content.
Writes are atomic. On POSIX, the directory is mode `0700` and file is mode
`0600`; Corum rejects a cache owned by another user or with group/other access.
On platforms without POSIX permission semantics, the file relies on the current
user profile's access controls and Corum displays this limitation in `jira
status`.

The cache is never written into a vault, copied by `corum init`, committed,
printed, or included in an exception. Logout removes the cache. Encrypting the
file with a key stored beside it is explicitly avoided because it would not add
a meaningful security boundary.

The initial version supports one active Atlassian account per operating-system
user. Logging in with another account replaces the cached session after an
explicit confirmation.

## Vault Configuration

Vault YAML remains secret-free. An OAuth-configured workspace records stable,
non-secret selection data:

```yaml
features:
  jira:
    enabled: true
jira:
  cloud_id: 01234567-89ab-cdef-0123-456789abcdef
  site: https://example.atlassian.net
  project: STUDY
  transitions:
    this_week: "2"
```

`cloud_id` identifies the selected Rovo MCP resource. `site` remains for human
readability and validation. The project key continues to scope all Jira work.

The workspace schema remains version 1 and gains an optional `cloud_id` field
for compatibility. A Jira-enabled command encountering an older configuration
without `cloud_id` stops before network mutation and directs the user to run
`corum jira login` in that vault to select a site and project.

## Jira MCP Adapter

The public plan and result models remain stable. A new adapter replaces the
direct REST client behind `apply_plan` and maps the existing operations to Rovo
MCP v2:

- epic and child reconciliation uses `getJiraIssue` and
  `searchJiraIssuesUsingJql`;
- creates use `createJiraIssue`;
- field updates use `editJiraIssue`; and
- transitions use `transitionJiraIssue`.

Every call includes the configured `cloud_id`. Existing safeguards remain:

- validate the complete exact plan before the first mutation;
- permit updates and transitions only for children of the configured epic;
- execute approved actions sequentially;
- record applied keys and partial failures;
- persist a reconciliation-required barrier after ambiguous writes; and
- atomically update Jira cache and run state.

Tool discovery and MCP response parsing stay inside the adapter. Unexpected
tool absence, schema changes, or malformed results become redacted domain
errors; they do not weaken the approval or retry boundary.

Dry-run validation does not require login and performs no MCP construction or
network access.

## Failure Handling

- Browser launch failure: print the URL and continue waiting for the callback.
- OAuth cancellation or timeout: return a concise error and write no vault.
- Invalid or inaccessible site/project: return to selection without writing
  configuration.
- Expired access token: let the SDK refresh it atomically.
- Revoked refresh token: stop before Jira mutation and instruct the user to run
  `corum jira login`.
- Rovo permission or admin-policy rejection: report the selected site and the
  missing permission group without exposing protocol payloads or tokens.
- MCP tool/schema drift: fail closed before mutation when detected during
  preflight; after a confirmed partial mutation, preserve reconciliation state.

## Dependencies

Add only the runtime libraries needed for the approved behavior:

- the official `mcp` Python SDK for Streamable HTTP and OAuth;
- `platformdirs` for portable per-user cache paths; and
- a Python-native interactive prompt library for the init form.

The implementation plan will pin compatible major-version ranges after a small
API compatibility check. No Go binary, Node runtime, browser driver, or local
web framework is required.

## Testing

Automated tests will cover:

- wizard branching, validation, cancellation, confirmation, and `--defaults`;
- callback binding, browser fallback, timeout, and state mismatch;
- atomic token storage, POSIX permissions, redaction, replacement, and logout;
- accessible-resource and project selection;
- each exact Jira action mapped to the expected MCP tool and `cloud_id`;
- missing tools, malformed results, revoked sessions, and ambiguous writes;
- preservation of dry-run isolation and Jira-disabled isolation;
- package installation with all new runtime dependencies; and
- migration behavior for version-1 configurations lacking `cloud_id`.

OAuth, browser, and MCP interactions are mocked in the normal suite. After the
automated suite passes, a single manual acceptance run will use a real Atlassian
account: the user completes the browser consent, selects a site/project, checks
`corum jira status`, and runs a read-only Jira reconciliation. No token or
authorization code is pasted into chat or test output.

## Security Boundary

This design protects credentials from vaults, source control, terminal output,
and agent-authored content. It does not claim isolation from malware or another
process already running as the same operating-system user. That stronger threat
model belongs to a future sandboxing project.
