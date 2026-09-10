# Scope Course Error Handling

Use this reference when scope inputs or prior Jira results require follow-up.

## Required inputs

Return `status: error` with a stable code and exact path for an unavailable required enabled input. Report successful source evidence and unresolved capture evidence from the selected manifest.

When the enabled Jira cache is absent, return `jira_cache_missing` with the `state/jira.json` path. `sync-course` initializes the cache through its empty-plan workflow, then invokes `scope-course` again with the selected manifest.

## Jira history

Use reconciled Jira `applied` and `failures` results as prior-write evidence. Use the refreshed cache as the current Jira state. Return a manual-reconciliation error when the cache cannot identify the result of an uncertain create.

## Service changes

Use the selected manifest’s `effective_features` values as the scope workflow states. A fresh capture creates the manifest snapshot after a Jira or wiki configuration change.
