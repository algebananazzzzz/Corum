# Sync Course Error Handling

Use this reference when capture, scoping, Jira application, wiki authoring, or review returns an unexpected result.

## Capture and scope

Record each capture failure as unknown source material with its source, path, and error. Report the capture result and retain its manifest as the run evidence. Use a fresh capture after a configuration change or after source access is restored.

Return scope errors with their stable code and path. Use the selected manifest as the source of service states and capture evidence.

## Jira results

Preserve the exact Jira command result in `state/latest-run.json`. For an interrupted or uncertain Jira write, reconcile the configured epic with this empty plan, then scope a fresh plan from the reconciled cache and present it for approval:

```json
{"version":2,"course":"{{COURSE}}","epic":"{{EPIC_KEY}}","actions":[]}
```

```console
corum jira apply {{COURSE}} < {{EMPTY_JIRA_PLAN_FILE}}
```

## Wiki results

Preserve completed pages, index entries, and prior ingestion records. Record sources in `state/wiki.json` when their planned pages, index entries, provenance markers, and review findings are complete. Report remaining page, source, and review work as follow-up items.

## Service changes

Run a fresh capture after enabling or disabling Jira or wiki. The new manifest supplies the service snapshot for the next scope and approval cycle. Existing Jira and wiki records remain available as course history.
