from __future__ import annotations

import logging
from contextlib import nullcontext
from datetime import UTC, datetime, timedelta
from types import SimpleNamespace
from typing import Any
from uuid import uuid4

import httpx
import pytest
from pydantic import SecretStr

from core.config import Settings
from jobs.execution import JobExecutionFailure
from notifications import executor as executor_module
from notifications import services as services_module
from notifications.executor import NotificationExecutor
from notifications.feishu import (
    FeishuDeliveryError,
    FeishuWebhook,
    report_card,
    sign,
    validate_webhook_url,
)
from notifications.models import NotificationDelivery
from notifications.schemas import DeliveryStatus
from notifications.services import NotificationService, delivery_operation_id
from reports.schemas import (
    DailyReportData,
    ReportComparison,
    ReportContentItem,
    ReportCoverage,
    ReportOverview,
    ReportSentiment,
)

NOW = datetime(2026, 9, 26, 1, tzinfo=UTC)
OWNER = uuid4()
TOPIC = uuid4()
REPORT = uuid4()
TARGET = uuid4()
DELIVERY = uuid4()
WEBHOOK = "https://open.feishu.cn/open-apis/bot/v2/hook/test-token"


def _data() -> DailyReportData:
    return DailyReportData(
        topic_id=TOPIC,
        topic_name="测试主题",
        window_start=NOW - timedelta(days=1),
        window_end=NOW,
        cutoff_at=NOW,
        overview=ReportOverview(
            posts=ReportComparison(current=4, previous=2, delta=2),
            comments=ReportComparison(current=8, previous=3, delta=5),
            platform_distribution={"hackernews": 4},
            sentiment_distribution={
                ReportSentiment.POSITIVE: 3,
                ReportSentiment.NEUTRAL: 1,
                ReportSentiment.NEGATIVE: 0,
            },
        ),
        top_contents=tuple(
            ReportContentItem(
                citation=f"c{index}",
                content_id=uuid4(),
                content_version_id=uuid4(),
                title=f"标题 {index}",
                summary="摘要",
                sentiment=ReportSentiment.POSITIVE,
                source_key="hackernews",
                url=f"https://example.com/posts/{index}",
                interaction_count=10,
                representative_comments=(),
            )
            for index in range(1, 5)
        ),
        risks=(),
        voices=(),
        coverage=ReportCoverage(sources=(), discovered_at_count=4, unanalyzed_count=0),
    )


def _settings(**overrides: Any) -> Settings:
    values: dict[str, Any] = {
        "database_url": "postgresql+psycopg://test:test@127.0.0.1:5432/test",
        "notifications_enabled": True,
        "feishu_webhook_url": SecretStr(WEBHOOK),
        "feishu_secret": SecretStr("private-signing-secret"),
    }
    values.update(overrides)
    return Settings(**values)


def test_feishu_card_and_signature_with_fake_transport(caplog: pytest.LogCaptureFixture) -> None:
    card = report_card(
        _data(),
        report_id=REPORT,
        generator="model",
        web_base_url="https://hotkey.example",
        vault_name="My Vault",
        export_relative_path="HotKey/日报/测试.md",
    )
    elements = card["card"]["elements"]
    assert card["msg_type"] == "interactive"
    assert "测试主题" in card["card"]["header"]["title"]["content"]
    assert "2026-09-26" in elements[0]["text"]["content"]
    assert "模型版" in elements[0]["text"]["content"]
    assert "相关帖子 4" in elements[1]["text"]["content"]
    assert len([item for item in elements if item["tag"] == "div"]) == 5
    assert "https://example.com/posts/3" in elements[4]["text"]["content"]
    buttons = elements[-1]["actions"]
    assert buttons[0]["url"] == f"https://hotkey.example/reports/{REPORT}"
    assert buttons[1]["url"] == (
        "obsidian://open?vault=My%20Vault&file=HotKey%2F%E6%97%A5%E6%8A%A5%2F%E6%B5%8B%E8%AF%95"
    )
    without_export = report_card(
        _data(),
        report_id=REPORT,
        generator="template",
        web_base_url="http://localhost:3000",
        vault_name="Vault",
        export_relative_path=None,
    )
    assert len(without_export["card"]["elements"][-1]["actions"]) == 1

    requests: list[dict[str, Any]] = []

    def respond(request: httpx.Request) -> httpx.Response:
        requests.append(__import__("json").loads(request.content))
        return httpx.Response(200, json={"code": 0})

    caplog.set_level(logging.INFO)
    with httpx.Client(transport=httpx.MockTransport(respond)) as client:
        FeishuWebhook(
            url=SecretStr(WEBHOOK), secret=SecretStr("private-signing-secret"), client=client
        ).send(card, now=NOW)
    assert len(requests) == 1
    assert requests[0]["timestamp"] == str(int(NOW.timestamp()))
    assert requests[0]["sign"] == sign(int(NOW.timestamp()), SecretStr("private-signing-secret"))
    assert requests[0]["sign"] == "lpaI/B0j7BTUTMosIKG+SYJs7MNL+1jvWBY2J91cXEk="
    assert "private-signing-secret" not in caplog.text
    assert "test-token" not in caplog.text


def test_webhook_url_rejects_lookalike_hosts_and_insecure_urls() -> None:
    for url in (
        "http://open.feishu.cn/open-apis/bot/v2/hook/secret",
        "https://open.feishu.cn.evil.example/open-apis/bot/v2/hook/secret",
        "https://open.feishu.cn@evil.example/open-apis/bot/v2/hook/secret",
    ):
        with pytest.raises(ValueError, match="invalid Feishu webhook URL"):
            validate_webhook_url(url)


@pytest.mark.parametrize(
    ("response", "error_code", "uncertain"),
    [
        (httpx.Response(400), "feishu_client_error", False),
        (httpx.Response(200, json={"code": 19021}), "feishu_api_error", False),
        (httpx.Response(500), "feishu_response_uncertain", True),
    ],
)
def test_feishu_rejects_non_success(
    response: httpx.Response, error_code: str, uncertain: bool
) -> None:
    with (
        httpx.Client(transport=httpx.MockTransport(lambda _: response)) as client,
        pytest.raises(FeishuDeliveryError) as caught,
    ):
        FeishuWebhook(url=SecretStr(WEBHOOK), secret=None, client=client).send(
            {"msg_type": "interactive", "card": {}}, now=NOW
        )
    assert caught.value.code == error_code
    assert caught.value.uncertain is uncertain
    assert "test-token" not in repr(caught.value)


class _DeliverySession:
    def __init__(self, delivery: NotificationDelivery) -> None:
        self.delivery = delivery

    def begin(self) -> Any:
        return nullcontext()

    def scalar(self, _statement: Any) -> NotificationDelivery:
        return self.delivery

    def flush(self) -> None:
        pass


def test_delivery_state_retries_three_times_and_never_retries_unknown() -> None:
    delivery = NotificationDelivery(
        id=DELIVERY,
        owner_id=OWNER,
        report_id=REPORT,
        report_version=1,
        target_id=TARGET,
        status="pending",
        attempt_count=0,
        last_error_code=None,
        sent_at=None,
        created_at=NOW,
        updated_at=NOW,
    )
    service = NotificationService(_DeliverySession(delivery), _settings())  # type: ignore[arg-type]
    for attempt in range(1, 4):
        assert (
            service.begin_sending(delivery_id=DELIVERY, owner_id=OWNER, now=NOW)[0]
            is DeliveryStatus.SENDING
        )
        assert delivery.attempt_count == attempt
        service.finish(
            delivery_id=DELIVERY,
            status=DeliveryStatus.FAILED,
            error_code="feishu_client_error",
            now=NOW,
        )
    assert service.begin_sending(delivery_id=DELIVERY, owner_id=OWNER, now=NOW)[1] is None
    assert delivery.attempt_count == 3
    delivery.status = "sending"
    assert (
        service.begin_sending(delivery_id=DELIVERY, owner_id=OWNER, now=NOW)[0]
        is DeliveryStatus.UNKNOWN
    )
    assert (
        service.begin_sending(delivery_id=DELIVERY, owner_id=OWNER, now=NOW)[0]
        is DeliveryStatus.UNKNOWN
    )


class _RowResult:
    def __init__(self, value: Any) -> None:
        self.value = value

    def mappings(self) -> Any:
        return self

    def one_or_none(self) -> Any:
        return self.value

    def scalar_one_or_none(self) -> Any:
        return self.value


class _ExecutorSession:
    def __enter__(self) -> _ExecutorSession:
        return self

    def __exit__(self, *_args: Any) -> None:
        pass

    def execute(self, statement: Any, _params: Any) -> _RowResult:
        if "FROM notification_deliveries" in str(statement):
            return _RowResult(
                {
                    "report_id": REPORT,
                    "report_version": 1,
                    "target_id": TARGET,
                    "channel": "feishu",
                    "enabled": True,
                    "secret_env": None,
                    "data": _data().model_dump(mode="json"),
                    "generator": "model",
                    "status": "final",
                    "topic_id": TOPIC,
                    "window_start": NOW,
                }
            )
        return _RowResult(None)

    def rollback(self) -> None:
        pass


@pytest.mark.parametrize(
    ("status_code", "timeout", "expected"),
    [(200, False, "succeeded"), (400, False, "failed"), (200, True, "unknown")],
)
def test_executor_fake_transport_does_not_resend_uncertain(
    monkeypatch: pytest.MonkeyPatch, status_code: int, timeout: bool, expected: str
) -> None:
    state = SimpleNamespace(status="pending", attempt_count=0)
    requests = 0

    class FakeNotificationService:
        def __init__(self, _session: Any, _settings: Settings) -> None:
            pass

        def begin_sending(self, **_kwargs: Any) -> tuple[DeliveryStatus, None]:
            if state.status in {"succeeded", "unknown"} or state.attempt_count >= 3:
                return DeliveryStatus(state.status), None
            state.status = "sending"
            state.attempt_count += 1
            return DeliveryStatus.SENDING, None

        def finish(self, *, status: DeliveryStatus, **_kwargs: Any) -> None:
            state.status = status.value

    monkeypatch.setattr(executor_module, "NotificationService", FakeNotificationService)
    monkeypatch.setattr(
        executor_module,
        "load_job_execution_configuration",
        lambda *_args, **_kwargs: SimpleNamespace(
            owner_id=OWNER,
            operation_id=delivery_operation_id(report_id=REPORT, version=1, target_id=TARGET),
            kind="notification.send",
            scope={"delivery_id": str(DELIVERY)},
        ),
    )

    def respond(_request: httpx.Request) -> httpx.Response:
        nonlocal requests
        requests += 1
        if timeout:
            raise httpx.ReadTimeout("request may have reached Feishu")
        return httpx.Response(status_code, json={"code": 0})

    message = SimpleNamespace(
        kind="notification.send",
        job_id=uuid4(),
        owner_id=OWNER,
        operation_id=delivery_operation_id(report_id=REPORT, version=1, target_id=TARGET),
        configuration_ref=f"report:{REPORT}",
        configuration_version=1,
    )
    with httpx.Client(transport=httpx.MockTransport(respond)) as client:
        executor = NotificationExecutor(
            lambda: _ExecutorSession(), _settings(), clock=lambda: NOW, client=client
        )  # type: ignore[arg-type]
        iterations = 3 if expected == "failed" else 1
        for _ in range(iterations):
            if expected == "failed":
                with pytest.raises(JobExecutionFailure):
                    executor.execute(message)
            else:
                executor.execute(message)
        executor.execute(message)
    assert state.status == expected
    assert state.attempt_count == iterations
    assert requests == iterations


def test_repeated_notification_scan_accepts_one_job(monkeypatch: pytest.MonkeyPatch) -> None:
    accepted: list[Any] = []

    class FakeJobService:
        def __init__(self, _session: Any, *, clock: Any) -> None:
            pass

        def accept_in_transaction(self, *, owner_id: Any, command: Any) -> None:
            accepted.append((owner_id, command))

    class FakeScanSession:
        def __init__(self) -> None:
            self.inserted = False

        def in_transaction(self) -> bool:
            return True

        def execute(self, statement: Any, _params: Any = None) -> Any:
            if "WITH first_final" in str(statement):
                return SimpleNamespace(
                    mappings=lambda: [
                        {
                            "report_id": REPORT,
                            "owner_id": OWNER,
                            "version": 1,
                            "target_id": TARGET,
                        }
                    ]
                )
            if "INSERT INTO notification_deliveries" in str(statement):
                delivery_id = None if self.inserted else DELIVERY
                self.inserted = True
                return SimpleNamespace(scalar_one_or_none=lambda: delivery_id)
            raise AssertionError("unexpected scan query")

    monkeypatch.setattr(services_module, "JobService", FakeJobService)
    session = FakeScanSession()
    service = NotificationService(session, _settings())  # type: ignore[arg-type]
    assert service.enqueue_due_in_transaction(now=NOW) == 1
    assert service.enqueue_due_in_transaction(now=NOW) == 0
    assert len(accepted) == 1
    assert accepted[0][1].operation_id == delivery_operation_id(
        report_id=REPORT, version=1, target_id=TARGET
    )
    disabled = _settings(notifications_enabled=False)
    assert NotificationService(session, disabled).enqueue_due_in_transaction(now=NOW) == 0  # type: ignore[arg-type]
