# Corum v1 final fix report

Date: 2026-09-06

Review base: `3063202`

Reviewed pre-fix head: `8eb153a`

Fix commit: `46bb07cbeafdf8f3a32f0ea3cf5cfd920bfee31b` (`fix: close Corum v1 final review gaps`)

Status: **READY** — all three Critical and all twelve Important findings are resolved. No block-before-merge finding remains open.

## Scope and rulings honored

- The public CLI remains exactly four top-level commands: `init`, `doctor`, `sync`, and `jira`.
- Wiki finalization is an explicit mode of the packaged `lint-wiki.py`; no fifth public command was added and Python still does not author prose.
- Run-manifest evolution is additive/backward-compatible: the original `status` and original required fields remain valid while rich identity, path, source, result, and recovery records are added.
- The strict discriminated Jira action union is unchanged.
- Disabled-feature isolation remains intact. Structural/secret safety checks still apply, but disabled Jira endpoint/project/epic semantics, credentials, clients, and network activity are skipped.
- Canvas uses the validated workspace IANA timezone. `Asia/Singapore` remains only the initialized example value.
- `pypdf` replaces the external `pdfinfo` program and both `pypdf` and `jsonschema` are installed runtime dependencies.
- The pre-existing untracked `.venv/` and `dist/` directories were used only as tooling/input and were not staged.

## Finding-by-finding disposition

| Finding | Root cause and data flow | Fix | Regression proof |
| --- | --- | --- | --- |
| Critical 1: Jira update/transition ownership | `_validate_plan` proved only key syntax and plan/config equality. An arbitrary syntactically valid key then flowed directly into `update_fields` or `transition_issue`. | Every non-empty apply now fetches the configured epic's authoritative children, normalizes the complete response, rejects every update/transition target outside that set, and only then writes the cache or performs a remote mutation. | `test_update_target_must_be_a_current_child_of_the_configured_epic_before_mutation`; sequential apply tests prove the ownership query precedes all writes. |
| Critical 2: Canvas HTTPS/auth boundary | Canvas accepted an HTTP/base-path host, the authenticated client followed redirects, and absolute pagination links could cross origins with the bearer header. | Workspace/client require a credential-free HTTPS origin. Authenticated API URLs, redirects, and pagination are same-origin checked before request. Redirects are followed manually. External HTTP(S) asset downloads stay allowed and receive no bearer header unless they are on the configured origin. | Host rejection, cross-origin pagination, credential-bearing pagination, cross-origin redirect, same-origin authenticated download, and unauthenticated external download tests in `tests/canvas/test_client.py`. |
| Critical 3: first Canvas sync | `read_canvas_state` unconditionally opened `state/canvas.json`, although a newly configured course has no machine-owned state yet. | The state layer returns a validated empty ledger for exactly the configured watched sources and writes it only through the normal non-dry sync path. | `test_missing_canvas_state_bootstraps_only_watched_sources_without_writing`; `test_first_sync_bootstraps_watched_canvas_state_in_the_state_layer`. |
| Important 1: exact Canvas manifest data | Capture functions collapsed item identity, path, dates, and action detail into free-form summaries. The scoper could not reliably map a manifest item back to raw evidence. | Added strict `Change`, `Failure`, and `CanvasSourceResult` records with stable IDs, item IDs, status, exact raw-relative paths, structured details, and per-source status/ID lists. Every capture function now emits them. Legacy minimal records normalize deterministically on read. | Rich file/announcement/page/module/syllabus assertions in `tests/canvas/test_sync.py`; legacy manifest normalization and schema tests in `tests/test_run.py`. |
| Important 2: Jira/wiki stage evidence | Downstream stages stored only `status`, so partial writes had no durable applied/failure or retry evidence. | `StageResult` now carries strict applied items, failures, write state, retry safety, reconciliation requirement, and reconciliation completion while retaining `status`. The published schema keeps new fields optional for old manifests. | Run-model/schema tests, Jira partial/reconciliation tests, and wiki finalization state assertions. |
| Important 3: announcement image failure/idempotence | The announcement was marked changed and Markdown was written before all images succeeded; its ledger did not advance, so retry selected a second unique filename. | The path is deterministic from date/title/item ID. All images must succeed before Markdown, change record, or ledger advancement. A failed capture emits one item-level failure with the same retry path. | `test_failed_announcement_image_is_partial_and_announcement_is_retryable`; `test_failed_announcement_retry_uses_the_same_deterministic_path`. |
| Important 4: verifier leakage | Raw `httpx` exceptions included request URLs, so `verifier` query values flowed through source failure strings into manifests/state/CLI output. | All Canvas HTTP status/request errors are converted at the client boundary to redacted domain errors. Malformed rejected download URLs are never echoed. | `test_download_http_error_redacts_canvas_verifier_from_exception`; `test_rejected_download_url_does_not_echo_its_verifier`. |
| Important 5: Jira missing key/partial recovery | A successful create without `key` raised `KeyError`; mutation, fetch, or cache failure discarded already-completed writes and gave retries no safe barrier. | Create response shape/key is validated and raises `JiraMutationError` with conservative write state. Apply records each confirmed mutation before fetch/cache, emits structured partial/failed results, persists the stage, returns nonzero from CLI, blocks non-empty work while reconciliation is required, and preserves evidence through exact empty-plan reconciliation (including repeated reconciliation). The skills require same-manifest reconciliation, fresh scoping, and fresh approval rather than old-plan replay or key guessing. | Missing-key client/apply/CLI tests; fetch/cache partial tests; non-empty retry barrier; evidence-preserving and repeated empty reconciliation tests in `tests/jira/test_apply.py`; recovery-routing assertions in `tests/test_agent_kit.py`. |
| Important 6: strict configuration/schema parity | Pydantic ignored unknown fields, published schemas allowed them, and free-form maps could hide credential keys. | All nested models forbid extras. YAML loading recursively rejects credential-like keys, including compound free-form keys. Runtime semantic validators can be skipped only for dormant Jira. Both config schemas are recursively closed and protect free-form property names. Nullable optional sections now match model serialization. | Recursive extra/credential tests in `tests/test_config.py`; valid round-trip plus model/schema rejection parity (including `vendor_api_key`) in `tests/test_schemas.py`. |
| Important 7: unsafe YAML frontmatter | Canvas-controlled scalar strings were interpolated directly into YAML lines, allowing type confusion or document-structure injection. | Frontmatter now serializes the complete mapping/list structure through a `SafeDumper`, preserving strings and Unicode. | `test_frontmatter_safe_serializes_canvas_controlled_scalars`. |
| Important 8: hard-coded Singapore time | Conversion and run-ID helpers used a module-global Singapore zone rather than workspace configuration. | Workspace timezone is validated with `ZoneInfo` and threaded through all Canvas timestamp/file-name/fetched/synced conversions and `RunManifest.create`. | New York conversion and run-ID regressions plus timezone propagation tests. |
| Important 9: lint findings exit zero | The linter printed findings but returned success, so pending/finalization callers could advance after an objective failure. | Findings return exit 2; invalid input/runtime failures return exit 1; clean lint or a committed exact finalization returns exit 0. Skills explicitly consume both nonzero states as failures. | Existing finding tests now assert exit 2; skill routing assertion checks the documented gate. |
| Important 10: atomic wiki finalization | The skill directly edited `wiki.json` and `latest-run.json`, with no exact payload/schema/current-run validation and no pair rollback. | Added `wiki-finalization.schema.json` and `lint-wiki.py --finalize`. It validates schema, current run/course/feature, exact manifest IDs and paths, dependencies, path containment/existence, raw readability/PDF pages, and full lint before committing. A per-course lock, same-directory scratch files, atomic replacements, and rollback keep the pair coherent on runtime failure. Failure-only results atomically update only the run stage and do not create wiki state. All skills prohibit direct edits. | Successful exact finalization, mismatched-source rejection/no mutation, failure-only state, and second-replace rollback tests. Clean-wheel finalizer smoke independently verified installed assets and both state files. |
| Important 11: undeclared PDF tool | PDF coverage called external `pdfinfo`, which was absent from package/runtime declarations. | Declared `pypdf>=5,<7`; the linter uses strict `PdfReader`; published schemas are also shipped in wheel data for finalization. | `test_pdf_lint_uses_declared_python_reader_without_pdfinfo_on_path`; corrupt/null-provenance PDF tests; clean venv reports `pypdf=6.17.0`. |
| Important 12: unrelated course validation | `corum sync COURSE` called whole-vault validation, parsing every course before selecting one. | Added selected-course discovery/validation that reads only requested course files. `--all` and `doctor` retain whole-vault validation. | `test_selected_course_sync_ignores_invalid_unrelated_course`. |

## RED/GREEN evidence

The review findings were verified against the pre-fix implementation before production edits. Focused regression batches produced these observed transitions:

```text
Initial repository baseline:
153 passed in 0.91s

Config / Canvas client / state / timezone regressions:
RED:   21 failed, 42 passed
GREEN: 63 passed in 0.30s

Rich manifest and Canvas capture regressions:
RED:   14 failed, 29 passed
GREEN: 43 passed

Jira ownership and partial-result regressions:
RED:   5 failed, 37 deselected
GREEN: 43 passed

Combined Canvas sync / Jira / run-state integration:
GREEN: 87 passed

Wiki finalizer / lint exit / Python PDF regressions:
RED:   6 failed, 25 deselected
GREEN: 6 passed
```

Additional one-test RED/GREEN loops were run for the non-empty Jira retry barrier, failure-only wiki state, skill finalizer routing, malformed verifier URL redaction, legacy manifest read normalization, reconciliation evidence preservation, repeated empty reconciliation, and model/schema nullable parity. The credential-bearing pagination regression initially demonstrated the old unsafe loop by hanging on its cyclic accepted link; that isolated test process was terminated, the URL boundary was fixed, and the test then passed. During final self-review, `vendor_api_key` exposed a remaining free-form model/schema mismatch (`1 failed, 12 passed`); adjacent compound-key detection fixed it and the focused configuration/schema gate finished `28 passed in 0.28s`.

## Final verification evidence

### Fresh full suite

Command:

```console
.venv/bin/python -m pytest -q
```

Exact output:

```text
........................................................................ [ 36%]
........................................................................ [ 73%]
.....................................................                    [100%]
197 passed in 3.51s
```

### Published schema validation

Command validates every `schemas/*.schema.json` with `Draft202012Validator.check_schema`.

Exact output:

```text
validated 8 JSON schemas
```

### Fresh isolated build

Command:

```console
.venv/bin/python -m build --outdir /tmp/corum-release-build-OJfDsP/dist
```

Exit: `0`. Final exact output:

```text
Successfully built corum-0.1.0.tar.gz and corum-0.1.0-py3-none-any.whl
```

The build log confirms `schemas/wiki-finalization.schema.json`, every other published schema, the agent kit, and the linter/finalizer were copied into both the sdist and wheel.

### Clean-wheel install and command surface

Commands used a newly created `/tmp/corum-release-final-VwxJHf/venv` and only the wheel from the fresh build.

```console
/tmp/corum-release-final-VwxJHf/venv/bin/pip install --quiet /tmp/corum-release-build-OJfDsP/dist/corum-0.1.0-py3-none-any.whl
/tmp/corum-release-final-VwxJHf/venv/bin/pip check
/tmp/corum-release-final-VwxJHf/venv/bin/python -c '...version checks...'
/tmp/corum-release-final-VwxJHf/venv/bin/corum --help
```

Exact output (the quiet install itself emitted no output and exited 0):

```text
No broken requirements found.
corum=0.1.0 jsonschema=4.26.0 pypdf=6.17.0
usage: corum [-h] {init,doctor,sync,jira} ...

positional arguments:
  {init,doctor,sync,jira}

options:
  -h, --help            show this help message and exit
```

### Clean-wheel init, doctor, sync, Jira-plan, and finalizer smoke

Initialization and empty-vault doctor output:

```text
/tmp/corum-release-final-VwxJHf/vault
Corum vault is valid (0 course(s))
```

After adding isolated smoke fixtures, the installed commands/finalizer all exited 0. Exact combined output:

```text
Corum vault is valid (2 course(s))
{
  "dry_run": true,
  "courses": [
    {
      "schema": 1,
      "run_id": "20260906T065052+0800",
      "course": "DEMO",
      "effective_features": {
        "jira": false,
        "wiki": true
      },
      "canvas": {
        "status": "up_to_date",
        "changes": [],
        "failures": [],
        "sources": []
      },
      "jira": {
        "status": "disabled",
        "applied": [],
        "failures": [],
        "reconciliation_required": false,
        "retry_safe": true,
        "reconciled": false
      },
      "wiki": {
        "status": "pending",
        "applied": [],
        "failures": [],
        "reconciliation_required": false,
        "retry_safe": true,
        "reconciled": false
      }
    }
  ]
}
{
  "schema": 1,
  "course": "JIRA",
  "epic": "DEMO-1",
  "actions": []
}
DEMO finalized
wiki-state/latest-run exact finalization verified
Corum vault is valid (2 course(s))
```

The state assertion proved exact contents of `wiki.json`, wiki stage `status: applied`, and stable finalization result identity in `latest-run.json`.

### Forbidden scans and whitespace

Scans covered the plan's MCP terms, legacy connector paths, external PDF/subprocess use, and hard-coded Canvas/run timezone aliases. Exact output:

```text
PASS: no forbidden MCP references
PASS: no external pdfinfo/subprocess dependency
PASS: no hard-coded Canvas/run timezone
PASS: no legacy connector paths
```

`git diff --check` and `git diff --cached --check` both exited 0 with no output before the code commit.

## Files and commit

Commit `46bb07cbeafdf8f3a32f0ea3cf5cfd920bfee31b` changes 30 files (`3149` insertions, `275` deletions):

- Runtime/package: `pyproject.toml`; `src/corum/{cli,config,run,state,validation,workspace}.py`; `src/corum/canvas/{client,convert,sync}.py`; `src/corum/jira/{apply,client}.py`.
- Schemas: `schemas/{corum,course,run-manifest,wiki-finalization}.schema.json`.
- Agent contract/docs: `README.md`, `agent-kit/AGENTS.base.md`, and the linting/scope/sync skills plus packaged linter.
- Regression tests: Canvas client/conversion/sync, Jira apply, agent-kit/finalizer, config, run state/manifests, and schema parity.

This report is committed separately so the implementation commit remains reviewable as one coherent final-fix wave.

## Self-review

- Re-read the complete final review and ledger rulings, then traced each finding from external input through validation, state, CLI, and skill consumers.
- Reviewed the staged diff by security boundary (configuration, Canvas auth/redirect/error handling, Jira ownership/mutation/recovery), compatibility boundary (models versus JSON schemas and legacy manifests), state ownership/transaction behavior, and installed asset resolution.
- Confirmed remote Jira ownership is established before the first remote mutation, failures after a write retain all known keys, and ambiguous writes cannot enter an automatic non-empty retry while reconciliation is required.
- Confirmed authenticated Canvas traffic cannot leave the configured HTTPS origin, while safe external asset downloads remain unauthenticated.
- Confirmed finalizer success is conditioned on exact current-manifest source identity/path, readable source, valid page references, clean lint, and valid existing output paths; failed dependencies cannot be finalized.
- Confirmed only the requested course is parsed for selected sync, while `doctor` and `--all` still validate the complete vault.
- Confirmed no generated package artifacts or virtual environment files entered the commit.

## Residual concerns / deferred minor findings

No Critical or Important finding remains.

The following review-classified safe-to-defer minors remain intentionally untouched:

- General atomic JSON writing can leave a scratch file if serialization itself raises before the cleanup scope.
- Draw.io strict validation still skips the previously identified one-token-label edge case.
- `doctor` does not yet cover every state/calendar/credential/tool prerequisite named by the broader design.
- The multi-course CLI JSON envelope has no separate published schema.
- `synced_at` still advances when every watched source fails.

The prior minor about exact manifest feature keys is closed by runtime validation, and the prior wheel-schema omission is closed because the finalizer requires installed schemas. Live Canvas/Jira services were not contacted; integration behavior is proven with mocked transports and clean-install local smokes, consistent with the plan.
