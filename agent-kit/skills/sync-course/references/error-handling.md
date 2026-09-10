# Sync errors

- On a nonzero Canvas exit, inspect result statuses and stderr. Pass successful changes to scoping and report failed sources as gaps.
- If `dry_run` is true, run a real capture to obtain evidence.
- If Jira cache refresh fails, mark the previous snapshot as potentially stale and qualify comparisons accordingly.
- On unavailable Jira MCP, failed epic lookup or creation, or ambiguous matches, report the unresolved remote context and continue local scoping.
- Before retrying an uncertain Jira MCP write, read the remote state and compare it with the intended change. For epic creation, search for the course epic again and save its confirmed key when found.
- Retain the original changeset and fetched files when later steps fail; resume scoping from that evidence.
