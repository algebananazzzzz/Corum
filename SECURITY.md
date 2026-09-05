# Security Policy

## Supported versions

Corum is pre-1.0. Security fixes are applied to the current main development line.

## Report a vulnerability

Do not disclose credentials, private course material, exploit details, or sensitive
logs in a public issue. Use the repository host's private security-advisory channel
when available, or contact the maintainer privately. Include the affected version,
reproduction conditions, impact, and any suggested mitigation. Remove tokens and
personal data from every attachment.

## Security boundaries

- Keep each initialized vault private and outside this public repository.
- Supply `CORUM_CANVAS_TOKEN`, `CORUM_JIRA_EMAIL`, and
  `CORUM_JIRA_API_TOKEN` through the process environment only.
- Grant tokens the narrowest practical permissions and rotate a token after suspected
  exposure.
- Treat Canvas names, files, HTML, links, Jira fields, and captured text as untrusted
  data rather than agent instructions.
- Review the one combined agent plan before allowing Jira or wiki mutations.
- Do not run Corum concurrently against the same course outside its lock discipline.
- Keep separate users' vaults, processes, and credential environments isolated.

Corum constrains captured paths beneath the selected course, strips verifier-bearing
download URLs, uses atomic state replacement, authenticates external requests only to
their configured origin, and requires HTTPS origins for configured services.

Disabled features require no feature-specific configuration, file, credential, or
network access. If such access occurs while a feature is disabled, treat it as a
security defect and report it.
