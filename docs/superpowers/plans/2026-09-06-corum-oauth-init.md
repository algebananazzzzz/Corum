# Corum OAuth Init Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Jira API-token authentication with portable browser OAuth through Atlassian Rovo MCP, and add a small interactive initialization flow that can select the user's Jira site and project.

**Architecture:** Keep Corum as a single local Python CLI. The official MCP SDK owns OAuth discovery, PKCE, token refresh, and Streamable HTTP; Corum supplies a loopback callback, a protected JSON token cache, a narrow Rovo session wrapper, and an adapter that preserves the existing `apply_plan` client boundary. Interactive prompts only collect configuration and call these services; deterministic initialization remains available through `--defaults`.

**Tech Stack:** Python 3.12, argparse, asyncio, Pydantic, PyYAML, `mcp>=2.1,<3`, `platformdirs>=4,<5`, `InquirerPy>=0.3,<0.4`, pytest, pytest-asyncio.

**Spec:** `docs/superpowers/specs/2026-09-06-corum-oauth-init-design.md`

## Global Constraints

- Jira authentication is OAuth-only. Delete the `CORUM_JIRA_EMAIL` and `CORUM_JIRA_API_TOKEN` path; do not add API-token fallback, Rovo CLI coupling, or keyring storage.
- Do not add a server, database, container, sandbox, browser driver, Node or Go helper, background daemon, or account-sharing feature.
- Use `https://mcp.atlassian.com/v2/mcp`; do not target the legacy v1 endpoint.
- OAuth secrets stay outside every vault and never appear in logs, exceptions, YAML, tests, or command output.
- Preserve the existing `JiraPlan`, `ApplyResult`, dry-run behavior, exact-plan approval boundary, sequential writes, ownership checks, cache reconciliation, and partial-write recovery behavior.
- `corum init --defaults` must be deterministic and must create a Jira-disabled vault without opening a browser.
- Automated tests mock browser, OAuth, and MCP boundaries. The only live activity is the explicit read-only compatibility checkpoint and final user-assisted acceptance.
- Keep schema version `1`; `jira.cloud_id` is optional for loading older vaults but mandatory before a real OAuth Jira operation.
- Make each task's listed commit before moving to the next task. Do not combine unrelated cleanup.

---

## Task 1: Add the minimal dependencies and secret-free workspace shape

**Files:**

- Modify: `pyproject.toml`
- Modify: `src/corum/config.py`
- Modify: `schemas/corum.schema.json`
- Modify: `src/corum/workspace.py`

- [ ] Add only the three approved runtime dependencies to `[project].dependencies`:

```toml
    "InquirerPy>=0.3,<0.4",
    "mcp>=2.1,<3",
    "platformdirs>=4,<5",
```

- [ ] Extend `JiraWorkspace` with the stable, non-secret resource identifier while keeping it optional for version-1 compatibility:

```python
class JiraWorkspace(_StrictModel):
    cloud_id: str | None = None
    site: HttpUrl
    project: str
    transitions: dict[str, str] = Field(default_factory=dict)

    @field_validator("cloud_id")
    @classmethod
    def cloud_id_is_nonblank(cls, value: str | None) -> str | None:
        if value is None:
            return None
        return require_nonblank(value, "Jira cloud ID")
```

Import `require_nonblank` from `corum.validation`. Do not impose UUID syntax because Atlassian resource IDs are opaque.

- [ ] Add optional `cloud_id` to the Jira object in `schemas/corum.schema.json` without adding it to `required`:

```json
"cloud_id": {"type": "string", "minLength": 1}
```

- [ ] Change `DEFAULT_WORKSPACE` to disable Jira and omit the example Jira block:

```python
DEFAULT_WORKSPACE = {
    "schema": 1,
    "workspace": {"timezone": "Asia/Singapore", "term": "AY2026/27 Semester 1"},
    "canvas": {"host": "https://canvas.example.edu"},
    "features": {"jira": {"enabled": False}, "wiki": {"enabled": True}},
    "calendar": {"timetable": "Timetable.md", "term": "Term_Calendar.md"},
}
```

- [ ] Let `initialize` accept a fully validated workspace mapping so the wizard can reuse the existing atomic scaffold boundary:

```python
def initialize(root: Path, workspace: WorkspaceConfig | dict | None = None) -> Path:
    root = root.resolve()
    if root.exists() and (not root.is_dir() or any(root.iterdir())):
        raise ValueError(f"refusing to initialize non-empty target: {root}")
    value = WorkspaceConfig.model_validate(workspace or DEFAULT_WORKSPACE)
    # Existing directory creation and agent-kit copies remain unchanged.
    (root / "corum.yaml").write_text(
        yaml.safe_dump(value.model_dump(mode="json", exclude_none=True), sort_keys=False),
        encoding="utf-8",
    )
```

Validate before the first filesystem write. The wizard calls `initialize` only after final confirmation, which is the required cancellation boundary.

- [ ] Run a focused import/config smoke check, deferring the full tests to Task 6:

```bash
python -m pip install -e '.[test]'
python -c 'from corum.config import JiraWorkspace; print(JiraWorkspace(site="https://example.atlassian.net", project="STUDY").cloud_id)'
python -c 'from corum.workspace import DEFAULT_WORKSPACE; assert DEFAULT_WORKSPACE["features"]["jira"]["enabled"] is False'
```

- [ ] Commit:

```bash
git add pyproject.toml src/corum/config.py schemas/corum.schema.json src/corum/workspace.py
git commit -m "feat: add OAuth workspace configuration"
```

---

## Task 2: Implement portable OAuth storage and the loopback callback

**Files:**

- Create: `src/corum/jira/auth.py`
- Create: `src/corum/jira/oauth_callback.py`

- [ ] In `src/corum/jira/auth.py`, implement the exact MCP `TokenStorage` protocol with one JSON file:

```python
from mcp.shared.auth import OAuthClientInformationFull, OAuthToken
from platformdirs import user_config_path


def auth_cache_path() -> Path:
    return user_config_path("corum", appauthor=False) / "auth.json"


class FileTokenStorage:
    def __init__(self, path: Path | None = None) -> None:
        self.path = (path or auth_cache_path()).expanduser()
```

The class implements `async get_tokens() -> OAuthToken | None`, `async
set_tokens(tokens: OAuthToken) -> None`, `async get_client_info() ->
OAuthClientInformationFull | None`, and `async
set_client_info(client_info: OAuthClientInformationFull) -> None`. The module
also exposes `clear_auth(path: Path | None = None) -> bool` and
`auth_exists(path: Path | None = None) -> bool`.

Store a versioned object whose values come from `model_dump(mode="json")`:

```json
{
  "schema": 1,
  "tokens": {},
  "client_info": {}
}
```

Each setter must preserve the other object. An absent file returns `None`; corrupt JSON or model validation raises a redacted `AuthCacheError` naming only the path.

- [ ] Make cache writes atomic and private:

```python
def _write_private_json(path: Path, value: dict[str, object]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    if os.name == "posix":
        os.chmod(path.parent, 0o700)
    fd, temporary = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    try:
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            json.dump(value, stream)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    except BaseException:
        Path(temporary).unlink(missing_ok=True)
        raise
```

Before every POSIX read or overwrite, reject a file not owned by `os.getuid()` or with any `stat.S_IRWXG | stat.S_IRWXO` bits. Do not include file contents in errors. `clear_auth` unlinks only the resolved auth file and returns whether it existed.

- [ ] In `src/corum/jira/oauth_callback.py`, build a standard-library loopback listener with no web framework:

```python
class LoopbackOAuthCallback:
    def __init__(
        self,
        timeout: float = 120.0,
        browser_open: Callable[[str], bool] = webbrowser.open,
    ) -> None:
        self.timeout = timeout
        self.browser_open = browser_open

    @property
    def redirect_uri(self) -> str:
        return f"http://127.0.0.1:{self.port}/callback"
```

The remaining public methods are `async redirect_handler(authorization_url:
str) -> None`, `async callback_handler() -> AuthorizationCodeResult`, and
`async aclose() -> None`, with the behavior below.

Bind `ThreadingHTTPServer(("127.0.0.1", 0), handler)` before constructing OAuth client metadata. The handler accepts only `GET /callback`, parses `code`, `state`, `iss`, and `error`, sends a short text response, and places the result onto an `asyncio.Future` through `loop.call_soon_threadsafe`. Unknown paths return 404. Multiple callbacks cannot overwrite the first result.

`redirect_handler` prints the URL before calling injected `browser_open(url)`. If browser opening returns false or raises, continue waiting and print one concise fallback message. `callback_handler` uses `asyncio.wait_for`; timeout and provider errors become `OAuthLoginError` without query strings or authorization codes.

Do not independently validate PKCE, state, or issuer here—the official SDK receives `AuthorizationCodeResult` and owns those validations.

- [ ] Run only syntax/type-shape smoke checks here:

```bash
python -m compileall -q src/corum/jira/auth.py src/corum/jira/oauth_callback.py
python -c 'from corum.jira.auth import FileTokenStorage; from corum.jira.oauth_callback import LoopbackOAuthCallback'
```

- [ ] Commit:

```bash
git add src/corum/jira/auth.py src/corum/jira/oauth_callback.py
git commit -m "feat: add portable OAuth session storage"
```

---

## Task 3: Add the narrow Rovo MCP session and Jira setup service

**Files:**

- Create: `src/corum/jira/rovo.py`
- Create: `src/corum/jira/setup.py`
- Modify: `src/corum/jira/__init__.py`

- [ ] In `src/corum/jira/rovo.py`, wrap only the MCP operations Corum needs:

```python
ROVO_MCP_URL = "https://mcp.atlassian.com/v2/mcp"


class RovoError(RuntimeError):
    pass


class AtlassianResource(BaseModel):
    id: str
    url: HttpUrl
    name: str


class JiraProject(BaseModel):
    key: str
    name: str


class RovoSession:
    def __init__(self, session: ClientSession) -> None:
        self._session = session
```

Expose `async list_tools() -> dict[str, Tool]`, `async call_json(name: str,
arguments: dict[str, object]) -> object`, `async user_info() -> dict[str,
object]`, `async resources() -> list[AtlassianResource]`, and `async
projects(cloud_id: str) -> list[JiraProject]`.

`call_json` must fail on `CallToolResult.isError`, concatenate only text content blocks, decode exactly one JSON value, and raise a redacted `RovoError` for missing tools, non-text results, invalid JSON, or an unexpected shape. It must never include raw MCP payloads in the error.

Normalize the currently documented shapes while accepting the common outer wrappers (`data`, `results`, `values`, `projects`) only inside this module. Sort resources by `(name.casefold(), id)` and projects by `(name.casefold(), key)` for stable prompts.

- [ ] Implement the async context manager that owns callback, OAuth provider, HTTP transport, and MCP session:

```python
@asynccontextmanager
async def open_rovo_session(
    storage: FileTokenStorage | None = None,
    *,
    browser_open: Callable[[str], bool] = webbrowser.open,
    timeout: float = 120.0,
    interactive: bool = True,
) -> AsyncIterator[RovoSession]:
    callback = LoopbackOAuthCallback(timeout=timeout, browser_open=browser_open)
    metadata = OAuthClientMetadata(
        client_name="Corum",
        redirect_uris=[AnyUrl(callback.redirect_uri)],
        grant_types=["authorization_code", "refresh_token"],
        response_types=["code"],
        token_endpoint_auth_method="none",
    )
    auth = OAuthClientProvider(
        server_url=ROVO_MCP_URL,
        client_metadata=metadata,
        storage=storage or FileTokenStorage(),
        redirect_handler=callback.redirect_handler,
        callback_handler=callback.callback_handler,
    )
    try:
        async with httpx.AsyncClient(auth=auth, follow_redirects=True) as client:
            async with streamable_http_client(ROVO_MCP_URL, http_client=client) as streams:
                async with ClientSession(streams[0], streams[1]) as session:
                    await session.initialize()
                    yield RovoSession(session)
    finally:
        await callback.aclose()
```

Use the concrete HTTP client class required by installed MCP 2.1 (`httpx` or its SDK alias) after verifying the import; do not duplicate OAuth protocol logic to avoid an import mismatch.
When `interactive=False`, provide redirect/callback handlers that raise
`LoginRequired("Jira session is missing or revoked; run corum jira login")`
without printing or opening an authorization URL. `jira status` and real Jira
apply use this mode; only `init` and `jira login` may launch a browser.

- [ ] In `src/corum/jira/setup.py`, keep site/project selection and YAML mutation separate from terminal prompts:

```python
@dataclass(frozen=True)
class JiraSelection:
    cloud_id: str
    site: str
    project: str


def configured_workspace(
    workspace: WorkspaceConfig,
    selection: JiraSelection,
) -> WorkspaceConfig:
    return workspace.model_copy(
        update={
            "features": workspace.features.model_copy(
                update={"jira": FeatureSwitch(enabled=True)}
            ),
            "jira": JiraWorkspace(
                cloud_id=selection.cloud_id,
                site=selection.site,
                project=selection.project,
                transitions=workspace.jira.transitions if workspace.jira else {},
            ),
        }
    )
```

Also implement `async available_jira_choices(session: RovoSession) ->
list[tuple[AtlassianResource, list[JiraProject]]]` and
`write_workspace_atomic(path: Path, workspace: WorkspaceConfig) -> None`.

`available_jira_choices` drops resources with no Jira projects but reports a clear error when none remain. `write_workspace_atomic` validates the final model, writes beside `corum.yaml`, `fsync`s, and replaces it. It never touches OAuth storage.

- [ ] Re-export only the stable Jira symbols needed by `cli.py` and `apply.py` from `src/corum/jira/__init__.py`.

- [ ] Run focused import checks, with tests still deferred:

```bash
python -m compileall -q src/corum/jira/rovo.py src/corum/jira/setup.py
python -c 'from corum.jira.rovo import ROVO_MCP_URL; assert ROVO_MCP_URL.endswith("/v2/mcp")'
```

- [ ] Commit:

```bash
git add src/corum/jira/rovo.py src/corum/jira/setup.py src/corum/jira/__init__.py
git commit -m "feat: add Atlassian Rovo OAuth session"
```

---

## Task 4: Add the init wizard and Jira session commands

**Files:**

- Create: `src/corum/prompts.py`
- Create: `src/corum/init_wizard.py`
- Modify: `src/corum/cli.py`

- [ ] Define a tiny injectable prompt boundary in `src/corum/prompts.py`; keep InquirerPy imports here so service modules remain testable without terminal patching:

The `Prompts` protocol has three synchronous methods: `text(message, default,
validate) -> str`, `confirm(message, default) -> bool`, and generic
`select(message, choices: Sequence[tuple[str, T]]) -> T`. Implement those
methods in `TerminalPrompts` by delegating to InquirerPy. Define
`PromptCancelled(ValueError)` as the sole cancellation error exposed outside
this module.

Convert `KeyboardInterrupt`, EOF, and InquirerPy cancellation into `PromptCancelled("setup cancelled")`. If ANSI rendering is unavailable, use InquirerPy's simple style and textual validation rather than adding a second UI framework.

- [ ] Implement `run_init_wizard` in `src/corum/init_wizard.py`:

```python
async def run_init_wizard(
    proposed_root: Path,
    prompts: Prompts,
    *,
    session_factory: RovoSessionFactory = open_rovo_session,
) -> Path:
    # Validate non-empty target before OAuth.
    # Collect path, timezone, term, Canvas origin, wiki toggle, Jira toggle.
    # If Jira is enabled, connect and select a resource/project.
    # Render a summary containing no OAuth values.
    # Call initialize only after final confirmation.
```

Construct a complete `WorkspaceConfig` with the existing calendar defaults. Validate timezone through `WorkspaceDetails` and Canvas origin through `CanvasWorkspace` as each answer is entered. If Jira is off, set `features.jira.enabled` false and `jira=None`. Cancellation and validation failures before confirmation must leave the target absent or unchanged.

- [ ] Add these parser shapes in `src/corum/cli.py`:

```text
corum init [path] [--defaults]
corum jira login [path]
corum jira status [path]
corum jira logout
corum jira apply COURSE [--dry-run]
```

The `path` default for login/status is `.`. `init` uses the wizard only when both stdin and stdout are TTYs and `--defaults` is absent. In a non-interactive terminal without `--defaults`, fail with `corum: interactive initialization requires a terminal; use --defaults`.

- [ ] Implement command handlers as small functions instead of extending the current `main` branch chain:

Implement `async _jira_login(vault: Path | None, prompts: Prompts) -> None`,
`async _jira_status(vault: Path | None) -> None`, and `_jira_logout() -> None`.

`jira login` opens `open_rovo_session`, reads the authenticated profile and sites/projects, and prints only the account display name/email returned by `atlassianUserInfo`. If the target contains `corum.yaml`, prompt for selection, print the proposed non-secret Jira YAML, and call `write_workspace_atomic` only after confirmation. If replacing an existing cached account, ask once before starting OAuth; cancellation preserves the old cache by first copying the original bytes to memory and restoring them atomically if login does not finish.

`jira status` must distinguish: no cache, cache present but invalid/unsafe, authenticated session, and session revoked. It opens the Rovo session with `interactive=False`, so status never launches a login browser. It prints the selected vault site/project when a valid `corum.yaml` exists. Never dump model objects or caught protocol responses.

`jira logout` calls `clear_auth()` and reports `Logged out` or `No Jira session` without changing a vault.

- [ ] Remove `_jira_credentials`, the `os` import used only for Jira secrets, and all references to `CORUM_JIRA_EMAIL` / `CORUM_JIRA_API_TOKEN` from CLI code.

- [ ] Run CLI help smoke checks:

```bash
corum init --help
corum jira login --help
corum jira status --help
corum jira logout --help
corum jira apply --help
```

- [ ] Commit:

```bash
git add src/corum/prompts.py src/corum/init_wizard.py src/corum/cli.py
git commit -m "feat: add interactive init and Jira login"
```

### Read-only live compatibility checkpoint

- [ ] With the user present, run `corum jira login <test-vault-or-empty-directory>` and let the user complete Atlassian consent in their own browser. Do not request or display a password, authorization code, access token, or refresh token.

- [ ] During that authenticated session, inspect `list_tools()` and record only these non-secret facts in implementation notes or test fixtures:

```text
exact tool names
required argument names and JSON types
outer result wrapper names
pagination cursor fields
```

Confirm `atlassianUserInfo`, `getAccessibleAtlassianResources`, `listJiraProjects`, `getJiraIssue`, `searchJiraIssuesUsingJql`, `createJiraIssue`, `editJiraIssue`, and `transitionJiraIssue` are exposed. Call only the first three plus a bounded read-only issue search such as `project = <selected key> ORDER BY updated DESC` with `maxResults: 1`. Do not create, edit, or transition anything.

- [ ] If the live schemas differ from Task 5's mapping, update Task 5 locally with the observed names before coding. If any required mutation tool is absent, stop and report the blocker; do not restore API-token authentication.

---

## Task 5: Replace the REST Jira client with the Rovo adapter

**Files:**

- Modify: `src/corum/jira/client.py`
- Modify: `src/corum/jira/apply.py`
- Modify: `src/corum/cli.py`

- [ ] Keep `JiraMutationError` and the five-method client contract consumed by `apply_plan`, but replace HTTP Basic Auth with an injected `RovoSession`:

```python
class JiraClient:
    def __init__(self, session: RovoSession, cloud_id: str) -> None:
        self._session = session
        self._cloud_id = require_nonblank(cloud_id, "Jira cloud ID")
```

Retain these exact async method signatures: `create_issue(fields) -> str`,
`update_fields(key, fields) -> None`, `transition_issue(key, transition) ->
None`, `fetch_issue(key) -> dict[str, Any]`, and `epic_children(epic) ->
list[dict[str, Any]]`.

- [ ] Map the existing normalized plan fields to the live-confirmed Rovo schemas. Use this documented baseline unless the checkpoint proves a different exact field name:

```python
await session.call_json("createJiraIssue", {
    "cloudId": cloud_id,
    "projectKey": fields["project"],
    "issueTypeName": fields["type"],
    "summary": fields["summary"],
    "description": fields.get("description"),
    "parent": fields.get("parent"),
    "additional_fields": {
        "duedate": fields.get("due"),
        "labels": fields.get("labels"),
    },
})

await session.call_json("editJiraIssue", {
    "cloudId": cloud_id,
    "issueIdOrKey": key,
    "fields": mapped_fields,
})

await session.call_json("transitionJiraIssue", {
    "cloudId": cloud_id,
    "issueIdOrKey": key,
    "transitionId": transition_id,
})

await session.call_json("getJiraIssue", {
    "cloudId": cloud_id,
    "issueIdOrKey": key,
    "fields": ["issuetype", "summary", "status", "duedate", "labels", "description", "updated"],
})

await session.call_json("searchJiraIssuesUsingJql", {
    "cloudId": cloud_id,
    "jql": f"parent = {json.dumps(epic_key)}",
    "fields": ["issuetype", "summary", "status", "duedate", "labels", "description", "updated"],
    "maxResults": 100,
    **cursor,
})
```

Omit unset optional values instead of sending null unless null is the explicitly approved field update. Keep Markdown-to-ADF conversion only if the live tool schema requires ADF; otherwise send Markdown text and delete the unused converter.

- [ ] Normalize create, issue, and search responses back into the shapes already consumed by `apply.py`. Validate every created key with `require_issue_key`. A create response without a provable key raises `JiraMutationError("successful Jira create response is missing a valid issue key", write_state="applied")`; a transport/tool failure whose application status is unknown raises `JiraMutationError("Jira mutation outcome is unknown", write_state="unknown")`. Validation or preflight failures use `write_state="not_applied"`.

- [ ] In `apply.py`, require `workspace.jira.cloud_id` in `_validate_plan` for non-dry-run operations while preserving old-vault dry-run validation. The smallest change is to add a `require_cloud_id: bool` parameter to `_validate_plan` and pass `not dry_run`. The error must instruct: `run corum jira login in this vault`.

- [ ] In CLI real apply, open one Rovo session for the whole plan and inject the adapter:

```python
async with open_rovo_session(interactive=False) as session:
    if workspace.jira is None or workspace.jira.cloud_id is None:
        raise ValueError("Jira OAuth configuration is incomplete; run corum jira login")
    client = JiraClient(session, workspace.jira.cloud_id)
    result = await apply_plan(vault, course, plan, client)
```

Do not construct the session for `--dry-run` or Jira-disabled courses.

- [ ] Delete REST-only imports and code (`httpx.BasicAuth`, REST URL construction, direct `/rest/api/3` calls). Keep `httpx` as a project dependency because Canvas and the MCP SDK path still use it.

- [ ] Run a compile/import smoke check:

```bash
python -m compileall -q src/corum/jira/client.py src/corum/jira/apply.py src/corum/cli.py
python -c 'from corum.jira.client import JiraClient, JiraMutationError'
```

- [ ] Commit:

```bash
git add src/corum/jira/client.py src/corum/jira/apply.py src/corum/cli.py
git commit -m "feat: route Jira plans through Rovo MCP"
```

---

## Task 6: Add concentrated regression coverage, documentation, and acceptance proof

**Files:**

- Create: `tests/jira/test_auth.py`
- Create: `tests/jira/test_oauth_callback.py`
- Create: `tests/jira/test_rovo.py`
- Create: `tests/jira/test_setup.py`
- Create: `tests/test_init_wizard.py`
- Modify: `tests/test_init.py`
- Modify: `tests/test_config.py`
- Modify: `tests/jira/test_apply.py`
- Modify: `tests/test_cli.py` if present; otherwise create it
- Modify: `README.md`

- [ ] Add auth-storage tests using an explicit `tmp_path / "config/auth.json"`; never inspect the developer's real cache. Cover:

```text
missing cache returns None
token and client registration round-trip independently
each setter preserves the other record
atomic replacement leaves valid JSON
POSIX directory is 0700 and file is 0600
POSIX foreign-owner or group/other-readable cache is rejected
corrupt JSON produces a redacted error without file contents
clear_auth removes only auth.json and is idempotent
```

- [ ] Add callback tests with a short timeout and an injected browser function. Cover the exact callback path, 404 on another path, code/state/issuer extraction, OAuth error, timeout, browser false/exception fallback, and first-callback-wins. Assert output contains the authorization URL but never callback query values.

- [ ] Add Rovo unit tests with a fake MCP session. Cover tool discovery, missing required tool, `isError`, invalid/mixed content, invalid JSON, resource/project wrapper normalization, deterministic sorting, and redacted failures. Assert every error string excludes fake access tokens and raw payload sentinels.

- [ ] Add setup tests for filtering sites without projects, preserving transitions, enabling Jira, optional legacy `cloud_id`, and atomic YAML replacement. Validate written YAML through both `WorkspaceConfig` and `schemas/corum.schema.json`.

- [ ] Add wizard/CLI tests with fake prompts and a fake async session factory. Cover:

```text
interactive Jira-disabled initialization
interactive Jira-enabled site/project selection
invalid timezone and Canvas origin are rejected before confirmation
final cancellation creates no files
OAuth cancellation creates no vault
--defaults creates a Jira-disabled vault and never opens OAuth
non-TTY init without --defaults gives the documented error
login updates only non-secret site/project/cloud_id after confirmation
login cancellation leaves existing YAML and auth bytes unchanged
status never prints token/client-registration fields
logout is idempotent
dry-run and Jira-disabled apply never open an OAuth session
real apply without cloud_id tells the user to run jira login
```

- [ ] Replace REST transport tests in `tests/jira/test_apply.py` with exact adapter mapping tests. Keep all existing `RecordingClient`/`FailIfCalledClient` `apply_plan` tests unchanged except fixture workspace data now includes `cloud_id` for real-operation cases. Cover all five methods, pagination, omitted optionals, explicit field clearing, tool failures, malformed create result, and the `write_state` classification used by reconciliation.

- [ ] Update `README.md` with only user-facing behavior:

```text
Jira works with Atlassian Cloud Free; no paid subscription is required for a small personal/friends setup.
Run corum init or corum jira login and finish consent in the browser.
Credentials live in the platform user config directory, outside the vault; protect that OS account and do not commit the cache.
Run corum jira status and corum jira logout to inspect or clear the local session.
CORUM_CANVAS_TOKEN remains required for Canvas sync.
There is no CORUM_JIRA_EMAIL or CORUM_JIRA_API_TOKEN setup.
Sandboxing/multi-user hosting is intentionally outside this local-first release.
```

- [ ] Run the focused new suite first:

```bash
pytest -q tests/jira/test_auth.py tests/jira/test_oauth_callback.py tests/jira/test_rovo.py tests/jira/test_setup.py tests/test_init_wizard.py tests/test_init.py tests/test_config.py tests/jira/test_apply.py tests/test_cli.py
```

Expected: all selected tests pass with no live network or browser activity.

- [ ] Run the complete regression and package gates:

```bash
pytest -q
python -m build
python -m venv /tmp/corum-oauth-wheel-venv
/tmp/corum-oauth-wheel-venv/bin/pip install dist/corum-*.whl
/tmp/corum-oauth-wheel-venv/bin/corum --help
/tmp/corum-oauth-wheel-venv/bin/corum init --defaults /tmp/corum-oauth-wheel-vault
/tmp/corum-oauth-wheel-venv/bin/corum doctor /tmp/corum-oauth-wheel-vault
```

Expected: the full suite passes; the wheel installs with all runtime dependencies; the installed CLI creates and validates a Jira-disabled vault. Remove only these explicit `/tmp/corum-oauth-wheel-*` paths after checking their resolved targets.

- [ ] Perform final user-assisted acceptance with the installed code:

```bash
corum jira login <user-chosen-vault>
corum jira status <user-chosen-vault>
corum jira apply <COURSE> --dry-run < approved-plan.json
```

The user completes browser consent. Verify status shows the intended account/site/project and that dry-run makes no Jira call. Then run one read-only reconciliation against the selected epic. Perform a create/edit/transition smoke test only if the user explicitly approves the exact plan and target project; otherwise the automated adapter tests plus read-only reconciliation are the completion gate.

- [ ] Search for forbidden legacy/auth leakage strings:

```bash
rg -n 'CORUM_JIRA_(EMAIL|API_TOKEN)|BasicAuth|/rest/api/3|mcp\.atlassian\.com/v1|access_token|refresh_token' src tests README.md schemas
```

Expected: no legacy Jira auth/REST/v1 references. Token field names may appear only inside `src/corum/jira/auth.py` serialization and synthetic auth tests, never in output formatting.

- [ ] Commit:

```bash
git add tests README.md
git commit -m "test: verify OAuth-first Jira workflow"
```

---

## Final Review and Integration

- [ ] Review `git diff main..HEAD` against the design spec. Reject any server, sandbox, keyring, API-token fallback, vault credential, or unrelated wiki change.
- [ ] Confirm `git status --short` is clean and `git log --oneline main..HEAD` contains the planned focused commits.
- [ ] Use `superpowers:requesting-code-review` for one final diff review, address only concrete in-scope findings, then rerun the affected focused tests and the complete `pytest -q` gate.
- [ ] Use `superpowers:verification-before-completion` before stating that the feature works.
- [ ] Use `superpowers:finishing-a-development-branch` to merge `feat/oauth-init` into `main`, push the public repository, and remove any temporary worktree only after verification. This branch is currently in the main checkout, so do not create a worktree solely to remove it later.
