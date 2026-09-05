from __future__ import annotations

import io
import json
from pathlib import Path

import httpx
import pytest
import yaml
from jsonschema import Draft202012Validator, FormatChecker
from jsonschema import ValidationError as SchemaValidationError
from jsonschema import validate
from pydantic import ValidationError

from corum import cli
from corum.config import JiraWorkspace, load_course, load_workspace
from corum.jira import JiraClient
from corum.jira import cache
from corum.jira import apply as jira_apply
from corum.jira.apply import InvalidPlan, JiraDisabled, JiraPlan, apply_plan


SCHEMAS = Path(__file__).parents[2] / "schemas"


def write_workspace(root: Path, *, include_jira: bool = True) -> None:
    value = {
        "schema": 1,
        "workspace": {"timezone": "Asia/Singapore", "term": "AY2026/27 Semester 1"},
        "canvas": {"host": "https://canvas.example.edu"},
        "calendar": {"timetable": "Timetable.md", "term": "Term_Calendar.md"},
    }
    if include_jira:
        value["jira"] = {"site": "https://example.atlassian.net", "project": "STUDY"}
    (root / "corum.yaml").write_text(yaml.safe_dump(value))


def write_course(
    root: Path,
    code: str,
    *,
    features: dict | None = None,
    include_jira: bool = True,
) -> None:
    folder = root / "courses" / code
    folder.mkdir(parents=True)
    value = {"schema": 1, "code": code, "canvas": {"id": 1, "sources": []}}
    if features is not None:
        value["features"] = features
    if include_jira:
        value["jira"] = {"epic": "STUDY-1"}
    (folder / "course.yaml").write_text(yaml.safe_dump(value))


def fetched_issue(key: str, *, summary: str | None = None, status: str = "To Do") -> dict:
    return {
        "key": key,
        "fields": {
            "issuetype": {"name": "Task"},
            "summary": summary or f"Summary {key}",
            "status": {"name": status},
            "duedate": "2026-09-11",
            "labels": ["assessment"],
            "description": {
                "type": "doc",
                "version": 1,
                "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Details"}]}],
            },
            "updated": "2026-09-03T09:12:41+08:00",
        },
    }


class FailIfCalledClient:
    def __getattr__(self, name):
        async def fail(*args, **kwargs):
            raise AssertionError("Jira client must not be called")

        return fail


@pytest.mark.asyncio
async def test_disabled_jira_rejects_apply_before_credentials_are_read(tmp_path):
    write_workspace(tmp_path, include_jira=False)
    write_course(
        tmp_path,
        "CS3103",
        features={"jira": {"enabled": False}},
        include_jira=False,
    )
    course = load_course(tmp_path, "CS3103")
    plan = JiraPlan(schema=1, course="CS3103", epic="STUDY-1", actions=[])

    with pytest.raises(JiraDisabled):
        await apply_plan(tmp_path, course, plan, client=FailIfCalledClient())


@pytest.mark.asyncio
async def test_plan_must_match_configured_epic(tmp_path):
    write_workspace(tmp_path)
    write_course(tmp_path, "CS3103")
    course = load_course(tmp_path, "CS3103")
    plan = JiraPlan(schema=1, course="CS3103", epic="OTHER-1", actions=[])

    with pytest.raises(InvalidPlan, match="epic"):
        await apply_plan(tmp_path, course, plan, client=FailIfCalledClient())


@pytest.mark.asyncio
async def test_complete_plan_is_semantically_validated_before_first_client_call(tmp_path):
    write_workspace(tmp_path)
    write_course(tmp_path, "CS3103")
    course = load_course(tmp_path, "CS3103")
    plan = JiraPlan.model_validate(
        {
            "schema": 1,
            "course": "CS3103",
            "epic": "STUDY-1",
            "actions": [
                {"action": "update", "key": "STUDY-2", "set": {"due": "2026-09-12"}},
                {
                    "action": "create",
                    "issue": {
                        "type": "Task",
                        "parent": "OTHER-1",
                        "summary": "Wrong parent",
                    },
                },
            ],
        }
    )

    with pytest.raises(InvalidPlan, match="parent"):
        await apply_plan(tmp_path, course, plan, client=FailIfCalledClient())


@pytest.mark.asyncio
async def test_dry_run_validates_without_network_or_cache_writes(tmp_path):
    write_workspace(tmp_path)
    write_course(tmp_path, "CS3103")
    course = load_course(tmp_path, "CS3103")
    plan = JiraPlan.model_validate(
        {
            "schema": 1,
            "course": "CS3103",
            "epic": "STUDY-1",
            "actions": [
                {
                    "action": "create",
                    "issue": {"type": "Task", "parent": "STUDY-1", "summary": "A task"},
                }
            ],
        }
    )

    result = await apply_plan(
        tmp_path,
        course,
        plan,
        client=FailIfCalledClient(),
        dry_run=True,
    )

    assert result.dry_run is True
    assert result.applied == []
    assert not (tmp_path / "courses/CS3103/state/jira.json").exists()


class RecordingClient:
    def __init__(self):
        self.events: list[tuple] = []
        self.fetch_count: dict[str, int] = {}

    async def create_issue(self, fields):
        self.events.append(("create", fields))
        return "STUDY-2"

    async def update_fields(self, key, fields):
        self.events.append(("update", key, fields))

    async def transition_issue(self, key, transition):
        self.events.append(("transition", key, transition))

    async def fetch_issue(self, key):
        self.events.append(("fetch", key))
        count = self.fetch_count.get(key, 0)
        self.fetch_count[key] = count + 1
        status = "This Week" if key == "STUDY-2" and count else "To Do"
        return fetched_issue(key, summary="Created" if key == "STUDY-2" else "Updated", status=status)

    async def epic_children(self, epic):
        self.events.append(("children", epic))
        return []


@pytest.mark.asyncio
async def test_actions_apply_sequentially_fetch_results_and_atomically_upsert_cache(tmp_path):
    write_workspace(tmp_path)
    workspace = yaml.safe_load((tmp_path / "corum.yaml").read_text())
    workspace["jira"]["transitions"] = {"this_week": "2"}
    (tmp_path / "corum.yaml").write_text(yaml.safe_dump(workspace))
    write_course(tmp_path, "CS3103")
    course = load_course(tmp_path, "CS3103")
    cache.reconcile(
        tmp_path,
        course,
        {"epic": "STUDY-1", "reconciled_at": "2026-09-03T14:30:00+08:00", "complete": True, "issues": []},
    )
    plan = JiraPlan.model_validate(
        {
            "schema": 1,
            "course": "CS3103",
            "epic": "STUDY-1",
            "actions": [
                {
                    "action": "create",
                    "issue": {
                        "type": "Task",
                        "parent": "STUDY-1",
                        "summary": "Created",
                        "description": "Details",
                        "due": "2026-09-11",
                        "labels": ["assessment"],
                    },
                },
                {"action": "update", "key": "STUDY-3", "set": {"summary": "Updated"}},
                {"action": "transition", "key": "STUDY-2", "transition": "this_week"},
            ],
        }
    )
    client = RecordingClient()

    result = await apply_plan(tmp_path, course, plan, client=client)

    assert [entry.action for entry in result.applied] == ["create", "update", "transition"]
    assert client.events == [
        (
            "create",
            {
                "project": "STUDY",
                "type": "Task",
                "parent": "STUDY-1",
                "summary": "Created",
                "description": "Details",
                "due": "2026-09-11",
                "labels": ["assessment"],
            },
        ),
        ("fetch", "STUDY-2"),
        ("update", "STUDY-3", {"summary": "Updated"}),
        ("fetch", "STUDY-3"),
        ("transition", "STUDY-2", "2"),
        ("fetch", "STUDY-2"),
    ]
    stored = json.loads((tmp_path / "courses/CS3103/state/jira.json").read_text())
    assert [row["key"] for row in stored["issues"]] == ["STUDY-2", "STUDY-3"]
    assert stored["issues"][0]["status"] == "This Week"


@pytest.mark.asyncio
async def test_missing_cache_is_reconciled_from_every_epic_child_then_upserted(tmp_path):
    write_workspace(tmp_path)
    write_course(tmp_path, "CS3103")
    course = load_course(tmp_path, "CS3103")
    plan = JiraPlan.model_validate(
        {
            "schema": 1,
            "course": "CS3103",
            "epic": "STUDY-1",
            "actions": [{"action": "update", "key": "STUDY-2", "set": {"summary": "Changed"}}],
        }
    )

    class RecoveringClient(RecordingClient):
        async def epic_children(self, epic):
            self.events.append(("children", epic))
            return [fetched_issue("STUDY-3")]

    client = RecoveringClient()
    await apply_plan(tmp_path, course, plan, client=client)

    assert client.events == [
        ("update", "STUDY-2", {"summary": "Changed"}),
        ("fetch", "STUDY-2"),
        ("children", "STUDY-1"),
    ]
    stored = json.loads((tmp_path / "courses/CS3103/state/jira.json").read_text())
    assert [row["key"] for row in stored["issues"]] == ["STUDY-2", "STUDY-3"]


def test_plan_models_and_schema_reject_unknown_or_ambiguous_actions():
    schema = json.loads((SCHEMAS / "jira-plan.schema.json").read_text())
    valid = {
        "schema": 1,
        "course": "CS3103",
        "epic": "STUDY-1",
        "actions": [
            {
                "action": "create",
                "issue": {"type": "Task", "parent": "STUDY-1", "summary": "Task"},
            },
            {"action": "update", "key": "STUDY-2", "set": {"due": None}},
            {"action": "transition", "key": "STUDY-2", "transition": "this_week"},
        ],
    }
    invalid = {
        "schema": 1,
        "course": "CS3103",
        "epic": "STUDY-1",
        "actions": [
            {"action": "update", "key": "STUDY-2", "set": {"due": "2026-09-11"}, "issue": {}},
        ],
    }

    JiraPlan.model_validate(valid)
    validate(valid, schema)
    with pytest.raises(ValidationError):
        JiraPlan.model_validate(invalid)
    with pytest.raises(SchemaValidationError):
        validate(invalid, schema)


@pytest.mark.parametrize("field", ["type", "parent", "summary"])
def test_update_plan_rejects_null_for_fields_jira_cannot_clear(field):
    plan = {
        "schema": 1,
        "course": "CS3103",
        "epic": "STUDY-1",
        "actions": [{"action": "update", "key": "STUDY-2", "set": {field: None}}],
    }
    schema = json.loads((SCHEMAS / "jira-plan.schema.json").read_text())

    with pytest.raises(ValidationError):
        JiraPlan.model_validate(plan)
    with pytest.raises(SchemaValidationError):
        validate(plan, schema)


def test_jira_state_schema_accepts_normalized_state_and_rejects_course_owned_epic():
    schema = json.loads((SCHEMAS / "jira-state.schema.json").read_text())
    state = {
        "schema": 1,
        "reconciled_at": "2026-09-03T14:30:00+08:00",
        "issues": [
            {
                "key": "STUDY-2",
                "type": "Task",
                "summary": "Task",
                "status": "To Do",
                "due": None,
                "labels": [],
                "description": None,
                "updated_at": None,
            }
        ],
    }
    validate(state, schema)

    with pytest.raises(SchemaValidationError):
        validate({**state, "epic": "STUDY-1"}, schema)


@pytest.mark.asyncio
async def test_http_client_uses_basic_auth_rest_v3_adf_and_token_pagination():
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        assert request.headers["Authorization"].startswith("Basic ")
        path = request.url.path
        if request.method == "POST" and path == "/rest/api/3/issue":
            return httpx.Response(201, json={"key": "STUDY-2"})
        if request.method == "PUT":
            return httpx.Response(204)
        if path.endswith("/transitions"):
            return httpx.Response(204)
        if path == "/rest/api/3/issue/STUDY-2":
            return httpx.Response(200, json=fetched_issue("STUDY-2"))
        if path == "/rest/api/3/search/jql":
            payload = json.loads(request.content)
            if "nextPageToken" not in payload:
                return httpx.Response(200, json={"issues": [fetched_issue("STUDY-2")], "nextPageToken": "page-2"})
            assert payload["nextPageToken"] == "page-2"
            return httpx.Response(200, json={"issues": [fetched_issue("STUDY-3")]})
        raise AssertionError(f"unexpected request: {request.method} {request.url}")

    client = JiraClient(
        "https://example.atlassian.net",
        "student@example.com",
        "secret",
        transport=httpx.MockTransport(handler),
    )
    assert await client.create_issue(
        {
            "project": "STUDY",
            "type": "Task",
            "parent": "STUDY-1",
            "summary": "Task",
            "description": "**Deadline:** See [Canvas](https://canvas.example.edu).",
        }
    ) == "STUDY-2"
    await client.update_fields("STUDY-2", {"due": "2026-09-11"})
    await client.transition_issue("STUDY-2", "2")
    assert (await client.fetch_issue("STUDY-2"))["key"] == "STUDY-2"
    assert [issue["key"] for issue in await client.epic_children("STUDY-1")] == [
        "STUDY-2",
        "STUDY-3",
    ]

    create_payload = json.loads(requests[0].content)["fields"]
    assert create_payload["issuetype"] == {"name": "Task"}
    assert create_payload["parent"] == {"key": "STUDY-1"}
    description = create_payload["description"]
    assert description["type"] == "doc"
    assert description["content"][0]["content"][0] == {
        "type": "text",
        "text": "Deadline:",
        "marks": [{"type": "strong"}],
    }
    link_text = description["content"][0]["content"][2]
    assert link_text == {
        "type": "text",
        "text": "Canvas",
        "marks": [{"type": "link", "attrs": {"href": "https://canvas.example.edu"}}],
    }
    search_payloads = [
        json.loads(request.content)
        for request in requests
        if request.url.path == "/rest/api/3/search/jql"
    ]
    assert search_payloads[0]["jql"] == 'parent = "STUDY-1"'


@pytest.mark.parametrize(
    "site",
    [
        "http://example.atlassian.net",
        "https://example.atlassian.net/jira",
        "https://example.atlassian.net?tenant=other",
        "https://user@example.atlassian.net",
    ],
)
def test_client_rejects_non_https_or_non_origin_sites_before_authentication(site):
    def handler(request: httpx.Request) -> httpx.Response:
        raise AssertionError("invalid Jira sites must not reach an authenticated transport")

    with pytest.raises(ValueError, match="HTTPS origin"):
        JiraClient(site, "student@example.com", "secret", transport=httpx.MockTransport(handler))


@pytest.mark.asyncio
@pytest.mark.parametrize("operation", ["update", "transition", "fetch"])
async def test_client_rejects_traversal_issue_keys_before_authenticated_request(operation):
    def handler(request: httpx.Request) -> httpx.Response:
        raise AssertionError("invalid Jira keys must not reach an authenticated transport")

    client = JiraClient(
        "https://example.atlassian.net",
        "student@example.com",
        "secret",
        transport=httpx.MockTransport(handler),
    )
    with pytest.raises(ValueError, match="issue key"):
        if operation == "update":
            await client.update_fields("../myself", {"summary": "Unsafe"})
        elif operation == "transition":
            await client.transition_issue("../myself", "2")
        else:
            await client.fetch_issue("../myself")


@pytest.mark.asyncio
async def test_client_rejects_unsafe_project_parent_epic_and_transition_before_request():
    def handler(request: httpx.Request) -> httpx.Response:
        raise AssertionError("invalid Jira identifiers must not reach an authenticated transport")

    client = JiraClient(
        "https://example.atlassian.net",
        "student@example.com",
        "secret",
        transport=httpx.MockTransport(handler),
    )
    with pytest.raises(ValueError, match="project key"):
        await client.create_issue(
            {"project": "../MYSELF", "type": "Task", "parent": "STUDY-1", "summary": "Unsafe"}
        )
    with pytest.raises(ValueError, match="issue key"):
        await client.create_issue(
            {"project": "STUDY", "type": "Task", "parent": "../MYSELF", "summary": "Unsafe"}
        )
    with pytest.raises(ValueError, match="issue key"):
        await client.epic_children('../myself" OR project IS NOT EMPTY')
    with pytest.raises(ValueError, match="transition ID"):
        await client.transition_issue("STUDY-2", "")


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("project", "transitions", "match"),
    [
        ("", {"this_week": "2"}, "project"),
        ("STUDY", {"this_week": ""}, "transition"),
    ],
)
async def test_all_resolved_config_values_are_preflighted_before_first_action(
    tmp_path, monkeypatch, project, transitions, match
):
    write_workspace(tmp_path)
    write_course(tmp_path, "CS3103")
    course = load_course(tmp_path, "CS3103")
    workspace = load_workspace(tmp_path)
    unsafe_jira = JiraWorkspace.model_construct(
        site=workspace.jira.site,
        project=project,
        transitions=transitions,
    )
    monkeypatch.setattr(
        jira_apply,
        "load_workspace",
        lambda vault: workspace.model_copy(update={"jira": unsafe_jira}),
    )
    plan = JiraPlan.model_validate(
        {
            "schema": 1,
            "course": "CS3103",
            "epic": "STUDY-1",
            "actions": [
                {"action": "update", "key": "STUDY-2", "set": {"summary": "First"}},
                {"action": "transition", "key": "STUDY-2", "transition": "this_week"},
            ],
        }
    )

    with pytest.raises(InvalidPlan, match=match):
        await apply_plan(tmp_path, course, plan, client=FailIfCalledClient())


def _plan_schema_accepts(value: dict) -> bool:
    schema = json.loads((SCHEMAS / "jira-plan.schema.json").read_text())
    return not list(
        Draft202012Validator(schema, format_checker=FormatChecker()).iter_errors(value)
    )


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        (
            {
                "schema": 1,
                "course": "CS3103",
                "epic": "STUDY-1",
                "actions": [
                    {"action": "update", "key": "STUDY-2", "set": {"due": "2026-09-11"}}
                ],
            },
            True,
        ),
        ({"schema": True, "course": "CS3103", "epic": "STUDY-1", "actions": []}, False),
        ({"schema": 1.0, "course": "CS3103", "epic": "STUDY-1", "actions": []}, True),
        ({"schema": 1, "course": "   ", "epic": "STUDY-1", "actions": []}, False),
        ({"schema": 1, "course": "CS3103", "epic": "../MYSELF", "actions": []}, False),
        (
            {
                "schema": 1,
                "course": "CS3103",
                "epic": "STUDY-1",
                "actions": [{"action": "update", "key": "../myself", "set": {"due": None}}],
            },
            False,
        ),
        (
            {
                "schema": 1,
                "course": "CS3103",
                "epic": "STUDY-1",
                "actions": [
                    {
                        "action": "create",
                        "issue": {"type": "Task", "parent": "NOPE", "summary": "Task"},
                    }
                ],
            },
            False,
        ),
        (
            {
                "schema": 1,
                "course": "CS3103",
                "epic": "STUDY-1",
                "actions": [
                    {"action": "transition", "key": "STUDY-2", "transition": "   "}
                ],
            },
            False,
        ),
        (
            {
                "schema": 1,
                "course": "CS3103",
                "epic": "STUDY-1",
                "actions": [
                    {"action": "update", "key": "STUDY-2", "set": {"summary": "   "}}
                ],
            },
            False,
        ),
        (
            {
                "schema": 1,
                "course": "CS3103",
                "epic": "STUDY-1",
                "actions": [
                    {"action": "update", "key": "STUDY-2", "set": {"due": "2026-02-30"}}
                ],
            },
            False,
        ),
        (
            {
                "schema": 1,
                "course": "CS3103",
                "epic": "STUDY-1",
                "actions": [
                    {"action": "update", "key": "STUDY-2", "set": {"due": 0}}
                ],
            },
            False,
        ),
    ],
)
def test_plan_model_and_schema_have_bidirectional_acceptance_agreement(value, expected):
    try:
        JiraPlan.model_validate(value)
    except ValidationError:
        model_accepts = False
    else:
        model_accepts = True

    assert model_accepts is expected
    assert _plan_schema_accepts(value) is expected


def test_cli_dry_run_prints_validated_plan_without_credentials_or_client(tmp_path, monkeypatch, capsys):
    write_workspace(tmp_path)
    write_course(tmp_path, "CS3103")
    plan = {
        "schema": 1,
        "course": "CS3103",
        "epic": "STUDY-1",
        "actions": [],
    }
    monkeypatch.chdir(tmp_path)
    monkeypatch.setattr(cli.sys, "stdin", io.StringIO(json.dumps(plan)))
    monkeypatch.delenv("CORUM_JIRA_EMAIL", raising=False)
    monkeypatch.delenv("CORUM_JIRA_API_TOKEN", raising=False)

    class ForbiddenClient:
        def __init__(self, *args, **kwargs):
            raise AssertionError("dry-run must not construct a Jira client")

    monkeypatch.setattr(cli, "JiraClient", ForbiddenClient)
    assert cli.main(["jira", "apply", "CS3103", "--dry-run"]) == 0
    assert json.loads(capsys.readouterr().out) == plan
    assert not (tmp_path / "courses/CS3103/state/jira.json").exists()


def test_cli_disabled_jira_exits_before_reading_credential_environment(tmp_path, monkeypatch, capsys):
    write_workspace(tmp_path, include_jira=False)
    write_course(
        tmp_path,
        "CS3103",
        features={"jira": {"enabled": False}},
        include_jira=False,
    )
    monkeypatch.chdir(tmp_path)

    def explode():
        raise AssertionError("must not read Jira credentials")

    monkeypatch.setattr(cli, "_jira_credentials", explode)

    assert cli.main(["jira", "apply", "CS3103"]) == 1
    assert "Jira is disabled" in capsys.readouterr().out
