from __future__ import annotations

from collections.abc import Callable
from concurrent.futures import ThreadPoolExecutor
from datetime import UTC, datetime, timedelta
from threading import Event
from types import SimpleNamespace
from uuid import UUID, uuid4

import httpx
import pytest
from fastapi.testclient import TestClient
from sqlalchemy import select, text
from tests.conftest import authenticate_test_client, authenticated_owner_id
from tests.integration.test_keyword_discovery import _accepted_message
from tests.integration.test_monitor_topics import (
    _csrf_headers,
    _seed_network_budget,
    _topic_payload,
)
from tests.integration.test_topic_runs import _ready_topic
from tests.integration.test_topic_runs import monitor_topic_client as _topic_client  # noqa: F401

import content.discovery_execution as discovery_execution
from connections.presets import BILIBILI_PRESET, SOURCE_PRESETS
from connections.schemas import SourceEntryPoint
from connections.services import SourcePresetService
from content.comments import CommentRequestMeter, build_comment_job_acceptance
from content.comments_execution import CommentsExecutor
from content.discovery import KeywordRequestMeter
from content.discovery_execution import KeywordDiscoveryExecutor
from content.models import ContentRecord
from content.schemas import CommentCollectionRunInput
from core.errors import ApplicationError
from jobs.collection_topics import TOPIC_COLLECTION_SEQUENCE
from jobs.execution import (
    ExecutionLease,
    JobExecutionFailure,
    JobExecutionService,
    MessageReference,
)
from jobs.models import CoverageWindow, Job
from jobs.schemas import (
    CoverageTerminalEvidence,
    CoverageWindowInput,
    JobAcceptanceInput,
    JobFailureCategory,
    JobObservationContext,
)
from jobs.services import CoverageWindowService, JobService
from monitors.services import MonitorTopicService
from sources.adapters.hackernews import HackerNewsAdapter
from sources.adapters.mediacrawler import MediaCrawlerAdapter
from sources.contracts import (
    SourceCapability,
    SourcePage,
    SourcePageState,
    SourceRequest,
    SourceSort,
)
from worker.app import create_job_dispatcher
from worker.execution import IsolatedProcessResult, JobProcessOutcome


def _setup(
    client: TestClient, kind: str = "keyword.search"
) -> tuple[str, UUID, JobAcceptanceInput]:
    location = _ready_topic(client, source_keys=("hackernews",))
    response = client.post(
        location + "/runs",
        headers=_csrf_headers(client),
        json={"operation_id": str(uuid4()), "source_keys": ["hackernews"]},
    )
    assert response.status_code == 202, response.json()
    job_id = UUID(response.json()["sources"][0]["job_ids"][0])
    with client.app.state.session_factory() as session:
        job = session.get(Job, job_id)
        assert job is not None
        if kind == "source.comments":
            now = datetime.now(UTC)
            command = build_comment_job_acceptance(
                CommentCollectionRunInput(
                    operation_id=uuid4(),
                    configuration_ref=job.configuration_ref,
                    configuration_version=job.configuration_version,
                    source_key="hackernews",
                    connection_id=UUID(str(job.scope["connection_id"])),
                    connection_version=int(job.scope["connection_version"]),
                    post_external_id="100",
                    entry_point=SourceEntryPoint.MANUAL,
                    starts_at=now - timedelta(hours=1),
                    ends_at=now,
                    max_pages=3,
                    max_requests=4,
                )
            )
            owner_id = job.owner_id
            job_id = JobService(session).accept(owner_id=owner_id, command=command).id
        else:
            command = JobAcceptanceInput(
                operation_id=uuid4(),
                kind=kind,
                observation=JobObservationContext(
                    configuration_ref=job.configuration_ref,
                    configuration_version=job.configuration_version,
                    source_key="hackernews",
                    source_capability=SourceCapability.SEARCH,
                ),
                scope=dict(job.scope),
            )
    return location, job_id, command


def _transition(client: TestClient, location: str, *, resume: bool = False) -> None:
    # Identical timestamps intentionally prove lifecycle sequence, not time, is the fence.
    now = datetime.now(UTC)
    with client.app.state.session_factory() as session:
        owner = authenticated_owner_id(session)
        service = MonitorTopicService(session, clock=lambda: now)
        topic_id = UUID(location.rsplit("/", 1)[1])
        service.pause_topic(owner_id=owner, topic_id=topic_id)
        if resume:
            service.resume_topic(owner_id=owner, topic_id=topic_id)


@pytest.mark.parametrize("kind", ["keyword.search", "source.comments"])
@pytest.mark.parametrize("resume", [False, True])
def test_queued_collection_stops_before_adapter_even_after_resume(
    request: pytest.FixtureRequest, kind: str, resume: bool
) -> None:
    client: TestClient = request.getfixturevalue("_topic_client")
    location, job_id, _command = _setup(client, kind)
    _transition(client, location, resume=resume)
    factory = client.app.state.session_factory
    message = _accepted_message(factory.kw["bind"], job_id)
    with factory() as session:
        execution = JobExecutionService(session, lease_seconds=60)
        lease = execution.acquire(job_id=job_id, worker_id="paused-queue")
    executor_class = KeywordDiscoveryExecutor if kind == "keyword.search" else CommentsExecutor
    executor = executor_class(
        factory,
        lease_seconds=60,
        adapter_factory=lambda *_args: pytest.fail("paused job constructed an adapter"),
    )
    lease, completion = executor.execute(message, lease)
    with factory() as session:
        execution = JobExecutionService(session, lease_seconds=60)
        execution.complete(
            lease,
            message=MessageReference(message.message_id, "controlled", 0, 0),
            completion=completion,
        )
        job = session.get(Job, job_id)
        assert job is not None
        assert (job.status, job.requests_sent, job.items_saved) == ("cancelled", 0, 0)
        coverage = session.scalar(
            select(CoverageWindow).where(CoverageWindow.last_job_id == job_id)
        )
        assert coverage is not None
        assert (coverage.status, coverage.stop_reason) == ("partial", "cancelled")
        assert session.scalar(text("SELECT count(*) FROM resource_usage_attempts")) == 0
        # Redelivery acknowledges the durable cancellation without acquiring a new lease.
        assert execution.acknowledge_cancelled(
            job_id=job_id, message=MessageReference(uuid4(), "controlled", 0, 1)
        )


@pytest.mark.parametrize("kind", ["keyword.search", "source.comments"])
def test_each_request_fences_running_job_without_new_usage_and_new_job_can_run(
    request: pytest.FixtureRequest, kind: str
) -> None:
    client: TestClient = request.getfixturevalue("_topic_client")
    location, job_id, command = _setup(client, kind)
    factory = client.app.state.session_factory
    now = datetime.now(UTC)
    with factory() as session:
        lease = JobExecutionService(session, lease_seconds=60).acquire(
            job_id=job_id, worker_id="paused-running"
        )
        job = session.get(Job, job_id)
        assert job is not None
        owner = job.owner_id
        connection_id = UUID(str(job.scope["connection_id"]))
        connection_version = int(job.scope["connection_version"])
    meter_class = KeywordRequestMeter if kind == "keyword.search" else CommentRequestMeter
    with factory() as session:
        meter = meter_class(
            session,
            owner_id=owner,
            lease=lease,
            operation_id=message_operation(client, job_id),
            source_key="hackernews",
            connection_id=connection_id,
            connection_version=connection_version,
            component_key="collector.hackernews",
            max_requests=4,
            deadline_at=now + timedelta(seconds=45),
            lease_seconds=60,
        )
        assert meter.before_request(1)
        _transition(client, location, resume=True)
        assert not meter.before_request(2)
        assert not meter.before_request(2)
        # The already admitted request is settled; no new request/usage was reserved.
        meter.fail_pending()
    with factory() as session:
        old = session.get(Job, job_id)
        assert old is not None
        assert old.requests_sent == 1
        assert old.cancel_requested_at is not None
        assert session.scalar(text("SELECT count(*) FROM resource_usage_attempts")) == 1
        fresh = JobService(session).accept(
            owner_id=owner, command=command.model_copy(update={"operation_id": uuid4()})
        )
        fresh_lease = JobExecutionService(session, lease_seconds=60).acquire(
            job_id=fresh.id, worker_id="resumed-new-operation"
        )
        assert JobExecutionService(session, lease_seconds=60).begin_request(fresh_lease)[1]


def message_operation(client: TestClient, job_id: UUID) -> UUID:
    with client.app.state.session_factory() as session:
        job = session.get(Job, job_id)
        assert job is not None
        return job.operation_id


@pytest.mark.parametrize("pause_at", ["inflight", "next_page"])
def test_search_keeps_committed_material_and_stops_next_request(
    request: pytest.FixtureRequest, pause_at: str
) -> None:
    client: TestClient = request.getfixturevalue("_topic_client")
    location, job_id, _command = _setup(client)
    factory = client.app.state.session_factory
    requested: list[str] = []

    def respond(request: httpx.Request) -> httpx.Response:
        requested.append(request.url.path)
        if pause_at == "inflight":
            _transition(client, location, resume=True)
        return httpx.Response(
            200,
            json={
                "hits": [
                    {
                        "objectID": "100",
                        "title": "Brand 召回 controlled release",
                        "author": "author",
                        "created_at_i": int(datetime.now(UTC).timestamp()) - 60,
                        "url": "https://example.com/release",
                        "points": 1,
                        "num_comments": 0,
                    }
                ],
                "page": 0,
                "nbPages": 2,
                "nbHits": 2,
            },
        )

    class PausingSearchAdapter(HackerNewsAdapter):
        def fetch_page(self, request: SourceRequest) -> SourcePage:
            if pause_at == "next_page" and request.page_token is not None:
                _transition(client, location, resume=True)
            return super().fetch_page(request)

    with factory() as session:
        lease = JobExecutionService(session, lease_seconds=60).acquire(
            job_id=job_id, worker_id="inflight-search"
        )
    message = _accepted_message(factory.kw["bind"], job_id)
    lease, completion = KeywordDiscoveryExecutor(
        factory,
        lease_seconds=60,
        adapter_factory=lambda before, cancelled, max_requests, max_seconds: PausingSearchAdapter(
            before_request=before,
            cancelled=cancelled,
            max_requests=max_requests,
            max_seconds=max_seconds,
            transport=httpx.MockTransport(respond),
        ),
    ).execute(message, lease)
    with factory() as session:
        JobExecutionService(session, lease_seconds=60).complete(
            lease,
            message=MessageReference(message.message_id, "controlled", 0, 0),
            completion=completion,
        )
        job = session.get(Job, job_id)
        assert job is not None
        saved = 1 if pause_at == "next_page" else 0
        assert (job.status, job.requests_sent, job.items_saved) == ("cancelled", 1, saved)
        assert session.scalar(select(ContentRecord.external_id)) == ("100" if saved else None)
        coverage = session.scalar(
            select(CoverageWindow).where(CoverageWindow.last_job_id == job_id)
        )
        assert coverage is not None
        assert (coverage.status, coverage.stop_reason, coverage.page_count) == (
            "partial",
            "cancelled",
            saved,
        )
    assert requested == ["/api/v1/search_by_date"]


@pytest.mark.parametrize("legacy", [False, True])
def test_stale_failed_retry_is_hidden_and_rejected_without_outbox(
    request: pytest.FixtureRequest, legacy: bool
) -> None:
    client: TestClient = request.getfixturevalue("_topic_client")
    location, job_id, _command = _setup(client)
    factory = client.app.state.session_factory
    message = _accepted_message(factory.kw["bind"], job_id)
    with factory() as session:
        execution = JobExecutionService(session, lease_seconds=60)
        lease = execution.acquire(job_id=job_id, worker_id="failed-before-pause")
        execution.record_failure(
            lease,
            message=MessageReference(message.message_id, "controlled", 0, 0),
            failure=JobExecutionFailure(
                error_code="controlled_failure",
                category=JobFailureCategory.TRANSIENT,
                occurred_at=datetime.now(UTC),
                next_action="受控重试",
                manual_retry_allowed=True,
            ),
        )
    if legacy:
        with factory.begin() as session:
            job = session.get(Job, job_id)
            assert job is not None
            job.scope = {
                key: value for key, value in job.scope.items() if key != TOPIC_COLLECTION_SEQUENCE
            }
    else:
        _transition(client, location, resume=True)
    with factory() as session:
        owner = authenticated_owner_id(session)
        service = JobService(session)
        status = service.get_status(owner_id=owner, job_id=job_id)
        assert status.failure is not None and not status.failure.manual_retry_allowed
        with pytest.raises(ApplicationError, match="job_not_retryable"):
            service.request_retry(owner_id=owner, job_id=job_id)
        assert (
            session.scalar(
                text("SELECT count(*) FROM outbox_messages WHERE aggregate_id=:id"), {"id": job_id}
            )
            == 1
        )


def test_pause_does_not_block_acceptance_or_request_with_topic_row_locked(
    request: pytest.FixtureRequest,
) -> None:
    client: TestClient = request.getfixturevalue("_topic_client")
    location, job_id, command = _setup(client)
    factory = client.app.state.session_factory
    locked, proceed = Event(), Event()

    def pause_transaction() -> None:
        with factory.begin() as session:
            session.execute(
                text("SELECT id FROM monitor_topics WHERE id=:id FOR UPDATE"),
                {"id": UUID(location.rsplit("/", 1)[1])},
            )
            locked.set()
            assert proceed.wait(5)
        _transition(client, location, resume=True)

    with ThreadPoolExecutor(max_workers=1) as pool:
        pending = pool.submit(pause_transaction)
        try:
            assert locked.wait(5)
            with factory() as session:
                session.execute(text("SET LOCAL statement_timeout='2s'"))
                owner = authenticated_owner_id(session)
                accepted = JobService(session).accept_in_transaction(
                    owner_id=owner, command=command
                )
                session.commit()
                lease = JobExecutionService(session, lease_seconds=60).acquire(
                    job_id=accepted.id, worker_id="mvcc-request"
                )
                assert JobExecutionService(session, lease_seconds=60).begin_request(lease)[1]
        finally:
            proceed.set()
        pending.result(timeout=5)
    with factory() as session:
        assert JobExecutionService(session, lease_seconds=60).cancellation_requested(lease)
        old = session.get(Job, job_id)
        assert old is not None and old.requests_sent == 0


@pytest.mark.parametrize("transition", ["archive", "remove_sources"])
def test_archive_and_automatic_pause_fence_existing_jobs(
    request: pytest.FixtureRequest, transition: str
) -> None:
    client: TestClient = request.getfixturevalue("_topic_client")
    location, job_id, command = _setup(client)
    if transition == "archive":
        response = client.post(location + "/archive", headers=_csrf_headers(client))
    else:
        response = client.patch(
            location,
            headers=_csrf_headers(client),
            json={**_topic_payload(), "expected_version": 1, "source_keys": []},
        )
    assert response.status_code == 200, response.json()
    factory = client.app.state.session_factory
    with factory() as session:
        execution = JobExecutionService(session, lease_seconds=60)
        lease = execution.acquire(job_id=job_id, worker_id="topic-lifecycle")
        assert not execution.begin_request(lease)[1]
        owner = authenticated_owner_id(session)
        with pytest.raises(ApplicationError, match="topic_not_ready"):
            JobService(session).accept(owner_id=owner, command=command)
        assert session.scalar(text("SELECT count(*) FROM jobs")) == 1


def test_other_topics_owners_and_source_hotlists_keep_request_permission(
    request: pytest.FixtureRequest,
) -> None:
    client: TestClient = request.getfixturevalue("_topic_client")
    location, job_id, command = _setup(client)
    factory = client.app.state.session_factory
    with factory() as session:
        owner = authenticated_owner_id(session)
        hotlist = JobService(session).accept(
            owner_id=owner,
            command=command.model_copy(
                update={
                    "operation_id": uuid4(),
                    "kind": "source.hotlist",
                    "observation": command.observation.model_copy(
                        update={
                            "configuration_ref": "hotlist:hackernews",
                            "source_capability": SourceCapability.HOTLIST,
                        }
                    ),
                }
            ),
        )
    _other_location, same_owner_id, _other_command = _setup(client)
    second_owner = authenticate_test_client(client)
    with factory.begin() as session:
        _seed_network_budget(session, second_owner)
        SourcePresetService(session).apply_in_transaction(
            owner_id=second_owner, preset=SOURCE_PRESETS["hackernews"]
        )
    created = client.post(
        "/api/topics",
        headers=_csrf_headers(client),
        json={**_topic_payload(), "source_keys": ["hackernews"]},
    )
    assert created.status_code == 201, created.json()
    foreign_location = created.headers["location"]
    assert (
        client.post(foreign_location + "/resume", headers=_csrf_headers(client)).status_code == 200
    )
    response = client.post(
        foreign_location + "/runs",
        headers=_csrf_headers(client),
        json={"operation_id": str(uuid4()), "source_keys": ["hackernews"]},
    )
    assert response.status_code == 202, response.json()
    foreign_id = UUID(response.json()["sources"][0]["job_ids"][0])
    with factory() as session:
        # Another account cannot attach its collection to the first owner's topic.
        with pytest.raises(ApplicationError, match="topic_not_ready"):
            JobService(session).accept(owner_id=second_owner, command=command)
        MonitorTopicService(session).pause_topic(
            owner_id=owner, topic_id=UUID(location.rsplit("/", 1)[1])
        )
        execution = JobExecutionService(session, lease_seconds=60)
        blocked = execution.acquire(job_id=job_id, worker_id="paused-owner")
        assert not execution.begin_request(blocked)[1]
        for unaffected_id in (hotlist.id, same_owner_id, foreign_id):
            lease = execution.acquire(job_id=unaffected_id, worker_id="unaffected")
            assert execution.begin_request(lease)[1]


def test_legacy_queued_job_fails_closed_and_replay_does_not_rebind_lifecycle(
    request: pytest.FixtureRequest,
) -> None:
    client: TestClient = request.getfixturevalue("_topic_client")
    location, _seed_id, command = _setup(client)
    factory = client.app.state.session_factory
    with factory() as session:
        owner = authenticated_owner_id(session)
        accepted = JobService(session).accept(owner_id=owner, command=command)
    with factory.begin() as session:
        job = session.get(Job, accepted.id)
        assert job is not None
        job.scope = {
            key: value for key, value in job.scope.items() if key != TOPIC_COLLECTION_SEQUENCE
        }
    _transition(client, location, resume=True)
    with factory() as session:
        replay = JobService(session).accept(owner_id=owner, command=command)
        assert replay.id == accepted.id
        job = session.get(Job, replay.id)
        assert job is not None and TOPIC_COLLECTION_SEQUENCE not in job.scope
        execution = JobExecutionService(session, lease_seconds=60)
        lease = execution.acquire(job_id=replay.id, worker_id="legacy-redelivery")
        assert execution.cancellation_requested(lease)
        assert not execution.begin_request(lease)[1]


@pytest.mark.parametrize("kind", ["keyword.search", "source.comments"])
@pytest.mark.parametrize("started", [False, True])
@pytest.mark.parametrize("malformed", [False, True])
def test_supervised_worker_cancellation_records_coverage_and_settles_inflight_usage(
    request: pytest.FixtureRequest, kind: str, started: bool, malformed: bool
) -> None:
    client: TestClient = request.getfixturevalue("_topic_client")
    location, job_id, _command = _setup(client, kind)
    factory = client.app.state.session_factory
    message = _accepted_message(factory.kw["bind"], job_id)
    if malformed:
        with factory.begin() as session:
            job = session.get(Job, job_id)
            assert job is not None
            job.scope = {key: value for key, value in job.scope.items() if key != "scan_kind"}
    if not started:
        _transition(client, location, resume=True)

    class ControlledSupervisor:
        def run(
            self,
            _target: object,
            args: tuple[object, ...],
            *,
            cancellation_requested: Callable[[], bool],
            **_kwargs: object,
        ) -> IsolatedProcessResult:
            lease = args[1]
            assert isinstance(lease, ExecutionLease)
            if started:
                with factory() as session:
                    job = session.get(Job, job_id)
                    assert job is not None
                    meter_class = (
                        KeywordRequestMeter if kind == "keyword.search" else CommentRequestMeter
                    )
                    meter = meter_class(
                        session,
                        owner_id=job.owner_id,
                        lease=lease,
                        operation_id=message.operation_id,
                        source_key="hackernews",
                        connection_id=UUID(str(job.scope["connection_id"])),
                        connection_version=int(job.scope["connection_version"]),
                        component_key="collector.hackernews",
                        max_requests=4,
                        deadline_at=datetime.now(UTC) + timedelta(seconds=45),
                        lease_seconds=60,
                    )
                    assert meter.before_request(1)
                    # The child is killed before its response/usage cleanup can run.
                    _transition(client, location, resume=True)
            assert cancellation_requested()
            return IsolatedProcessResult(outcome=JobProcessOutcome.CANCELLED)

    dispatch = create_job_dispatcher(
        factory,
        {kind: lambda _context: pytest.fail("cancelled child handler ran")},
        worker_id="controlled-supervisor",
        lease_seconds=60,
        supervisor=ControlledSupervisor(),
        stopping=Event(),
    )
    reference = MessageReference(message.message_id, "controlled", 0, 0)
    dispatch(message, reference)
    # Redelivery must not reopen the child or charge an already settled attempt.
    dispatch(message, reference)
    with factory() as session:
        job = session.get(Job, job_id)
        assert job is not None
        assert (job.status, job.requests_sent) == ("cancelled", int(started))
        coverage = session.scalar(
            select(CoverageWindow).where(CoverageWindow.last_job_id == job_id)
        )
        if malformed:
            assert coverage is None
        else:
            assert coverage is not None
            assert (coverage.status, coverage.stop_reason, coverage.page_count) == (
                "partial",
                "cancelled",
                0,
            )
        attempts = (
            session.execute(text("SELECT outcome FROM resource_usage_attempts")).scalars().all()
        )
        assert attempts == (["failed"] if started else [])
        reservations = session.execute(
            text("SELECT status, requested_units, actual_units FROM resource_budget_reservations")
        ).all()
        assert all(
            status == "settled" and actual == requested
            for status, requested, actual in reservations
        )
        assert bool(reservations) == started
        windows = session.execute(
            text("SELECT used_units, reserved_units FROM resource_budget_windows")
        ).all()
        assert all(reserved == 0 for _used, reserved in windows)
        assert all(used == 1 for used, _reserved in windows)
        assert len(windows) == len(reservations)


@pytest.mark.parametrize("kind", ["keyword.search", "source.comments"])
@pytest.mark.parametrize("confirmed", [False, True])
def test_supervised_cancellation_preserves_committed_checkpoint_and_confirmed_window(
    request: pytest.FixtureRequest, kind: str, confirmed: bool
) -> None:
    client: TestClient = request.getfixturevalue("_topic_client")
    location, job_id, _command = _setup(client, kind)
    factory = client.app.state.session_factory
    message = _accepted_message(factory.kw["bind"], job_id)

    class ControlledSupervisor:
        def run(
            self,
            _target: object,
            args: tuple[object, ...],
            *,
            cancellation_requested: Callable[[], bool],
            **_kwargs: object,
        ) -> IsolatedProcessResult:
            lease = args[1]
            assert isinstance(lease, ExecutionLease)
            # Simulate the child's atomic page/checkpoint commit. The parent
            # still holds the acquired lease with checkpoint_sequence == 0.
            with factory.begin() as session:
                job = session.get(Job, job_id)
                assert job is not None
                scope = job.scope
                window = CoverageWindowInput(
                    owner_id=job.owner_id,
                    source_key="hackernews",
                    capability=SourceCapability.SEARCH
                    if kind == "keyword.search"
                    else SourceCapability.COMMENTS,
                    target_hash=bytes.fromhex(str(scope["target_hash"])),
                    sort_key=SourceSort(str(scope["sort_key"])),
                    rule_version=int(scope["rule_version"]),
                    starts_at=datetime.fromisoformat(str(scope["starts_at"])),
                    ends_at=datetime.fromisoformat(str(scope["ends_at"])),
                )
                execution = JobExecutionService(session, lease_seconds=60)
                coverage = CoverageWindowService(session, execution=execution)
                coverage.begin_in_transaction(lease=lease, window=window)
                updated = execution.save_checkpoint_in_transaction(
                    lease, sequence=1, checkpoint={"page": 1}
                )
                result = coverage.record_page_in_transaction(
                    lease=updated,
                    window=window,
                    page_state=SourcePageState.COMPLETE if confirmed else SourcePageState.MORE,
                    evidence=CoverageTerminalEvidence(
                        starts_at=window.starts_at,
                        ends_at=window.ends_at,
                        sort_key=window.sort_key,
                        query_bounded=True,
                        sort_applied=True,
                        terminal_verified=True,
                    )
                    if confirmed
                    else None,
                )
                assert result.page_count == 1
            _transition(client, location, resume=True)
            assert cancellation_requested()
            return IsolatedProcessResult(outcome=JobProcessOutcome.CANCELLED)

    create_job_dispatcher(
        factory,
        {kind: lambda _context: pytest.fail("unexpected child execution")},
        worker_id="controlled-checkpoint",
        lease_seconds=60,
        supervisor=ControlledSupervisor(),
        stopping=Event(),
    )(message, MessageReference(message.message_id, "controlled", 0, 0))
    with factory() as session:
        job = session.get(Job, job_id)
        assert job is not None
        assert (job.status, job.checkpoint_sequence) == ("cancelled", 1)
        window = session.scalar(select(CoverageWindow).where(CoverageWindow.last_job_id == job_id))
        assert window is not None
        assert (window.page_count, window.checkpoint_sequence) == (1, 1)
        assert (window.status, window.stop_reason) == (
            ("confirmed", None) if confirmed else ("partial", "cancelled")
        )


def test_already_accepted_mediacrawler_search_is_rejected_before_budget_or_child_start(
    request: pytest.FixtureRequest,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    client: TestClient = request.getfixturevalue("_topic_client")
    _location, _seed_id, command = _setup(client)
    factory = client.app.state.session_factory
    with factory() as session:
        owner = authenticated_owner_id(session)
        with session.begin():
            connection = SourcePresetService(session).apply_in_transaction(
                owner_id=owner,
                preset=BILIBILI_PRESET,
            )
        scope = dict(command.scope)
        scope.update(
            {
                "connection_id": str(connection.connection_id),
                "connection_version": connection.connection_version,
                "page_size": 2,
                "max_pages": 1,
                "manual_source_keys": "bilibili",
            }
        )
        accepted = JobService(session).accept(
            owner_id=owner,
            command=command.model_copy(
                update={
                    "scope": scope,
                    "observation": command.observation.model_copy(
                        update={"source_key": "bilibili"}
                    ),
                }
            ),
        )
        lease = JobExecutionService(session, lease_seconds=60).acquire(
            job_id=accepted.id,
            worker_id="legacy-crawler",
        )
    message = _accepted_message(factory.kw["bind"], accepted.id)
    monkeypatch.setattr(
        discovery_execution, "get_settings", lambda: SimpleNamespace(mediacrawler_enabled=True)
    )
    monkeypatch.setattr(
        MediaCrawlerAdapter,
        "_run_child",
        lambda *_args: pytest.fail("unsafe crawler child launched"),
    )
    monkeypatch.setattr(
        KeywordRequestMeter,
        "before_request",
        lambda *_args: pytest.fail("unsafe crawler reserved requests"),
    )
    with pytest.raises(JobExecutionFailure) as failure:
        KeywordDiscoveryExecutor(factory, lease_seconds=60).execute(message, lease)
    assert failure.value.error_code == "search_request_guard_unavailable"
    assert not failure.value.manual_retry_allowed
    with factory() as session:
        JobExecutionService(session, lease_seconds=60).record_failure(
            lease,
            message=MessageReference(message.message_id, "controlled", 0, 0),
            failure=failure.value,
        )
        status = JobService(session).get_status(owner_id=owner, job_id=accepted.id)
        assert status.failure is not None and not status.failure.manual_retry_allowed
        assert (
            session.scalar(text("SELECT requests_sent FROM jobs WHERE id=:id"), {"id": accepted.id})
            == 0
        )
        assert session.scalar(text("SELECT count(*) FROM resource_usage_attempts")) == 0
        assert session.scalar(text("SELECT count(*) FROM resource_budget_reservations")) == 0
