# Sync errors

- A nonzero Canvas exit can still include valid JSON results for completed
  courses or sources. Read both stdout and stderr. Do not discard completed
  captures or treat failed sources as empty.
- A dry run reports readiness without fetching Canvas changes. Run a real
  capture to obtain evidence.
- If Jira sync fails, the prior complete issue cache remains unchanged. Report
  that it may be stale and do not claim it reflects the remote epic.
- If a Jira MCP write has an uncertain outcome, read the remote epic again and
  compare with the intended change before attempting a retry. Do not blindly
  recreate an issue.
- Keep fetched files as evidence even when later planning or remote writes fail.
  There is no workflow log or automatic replay queue.
