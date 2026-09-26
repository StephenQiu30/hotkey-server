from __future__ import annotations

import os
from collections.abc import Callable
from datetime import UTC, datetime, timedelta
from pathlib import Path
from uuid import UUID

import httpx
from pydantic import SecretStr
from sqlalchemy import text
from sqlalchemy.orm import Session, sessionmaker

from core.config import Settings
from jobs.execution import JobCompletion, JobExecutionFailure
from jobs.schemas import JobFailureCategory, JobMessage, JobStatus
from jobs.services import load_job_execution_configuration
from knowledge.services import daily_object_id
from notifications.feishu import FeishuDeliveryError, FeishuWebhook, report_card
from notifications.schemas import DeliveryStatus
from notifications.services import NotificationService, delivery_operation_id
from reports.schemas import DailyReportData


class NotificationExecutor:
    def __init__(
        self,
        sessions: sessionmaker[Session],
        settings: Settings,
        *,
        clock: Callable[[], datetime] | None = None,
        client: httpx.Client | None = None,
    ) -> None:
        self._sessions = sessions
        self._settings = settings
        self._clock = clock or (lambda: datetime.now(UTC))
        self._client = client

    def execute(self, message: JobMessage) -> JobCompletion:
        if message.kind != "notification.send":
            raise ValueError("notification executor received another task kind")
        with self._sessions() as session:
            configuration = load_job_execution_configuration(session, job_id=message.job_id)
            if (
                configuration is None
                or configuration.owner_id != message.owner_id
                or configuration.operation_id != message.operation_id
                or configuration.kind != message.kind
            ):
                raise ValueError("notification job configuration mismatch")
            delivery_id = UUID(str(configuration.scope["delivery_id"]))
            row = (
                session.execute(
                    text("""
                SELECT d.report_id, d.report_version, d.target_id, t.channel,
                       t.enabled, t.secret_env, r.data, r.generator, r.status,
                       r.topic_id, r.window_start
                FROM notification_deliveries d
                JOIN notification_targets t ON t.id = d.target_id AND t.owner_id = d.owner_id
                JOIN reports r ON r.id = d.report_id AND r.owner_id = d.owner_id
                WHERE d.id = :delivery_id AND d.owner_id = :owner_id
            """),
                    {"delivery_id": delivery_id, "owner_id": message.owner_id},
                )
                .mappings()
                .one_or_none()
            )
            if (
                row is None
                or message.operation_id
                != delivery_operation_id(
                    report_id=row["report_id"],
                    version=row["report_version"],
                    target_id=row["target_id"],
                )
                or message.configuration_ref != f"report:{row['report_id']}"
                or message.configuration_version != row["report_version"]
            ):
                raise ValueError("notification delivery does not match job")
            if row["status"] != "final" or row["channel"] != "feishu":
                raise ValueError("notification report or channel is unavailable")
            data = DailyReportData.model_validate(row["data"])
            export_id = daily_object_id(
                owner_id=message.owner_id,
                topic_id=row["topic_id"],
                window_start=row["window_start"],
            )
            export_path = session.execute(
                text("""
                SELECT relative_path FROM knowledge_exports
                WHERE owner_id = :owner_id AND object_type = 'daily' AND object_id = :object_id
            """),
                {"owner_id": message.owner_id, "object_id": export_id},
            ).scalar_one_or_none()
            session.rollback()
            if not self._settings.notifications_enabled or not row["enabled"]:
                raise JobExecutionFailure(
                    error_code="notification_target_disabled",
                    category=JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                    occurred_at=self._clock(),
                    next_action="启用通知和投递目标后手动重试",
                    manual_retry_allowed=True,
                )
            status, _ = NotificationService(session, self._settings).begin_sending(
                delivery_id=delivery_id, owner_id=message.owner_id, now=self._clock()
            )
        if status is not DeliveryStatus.SENDING:
            return JobCompletion(status=JobStatus.SUCCEEDED)
        try:
            card = report_card(
                data,
                report_id=row["report_id"],
                generator=row["generator"],
                web_base_url=self._settings.web_base_url,
                vault_name=Path(self._settings.obsidian_vault_path).name,
                export_relative_path=export_path,
            )
            webhook = self._settings.feishu_webhook_url
            if webhook is None or not webhook.get_secret_value():
                raise FeishuDeliveryError("feishu_webhook_unconfigured")
            secret: SecretStr | None
            if row["secret_env"]:
                raw_secret = os.environ.get(row["secret_env"])
                if not raw_secret:
                    raise FeishuDeliveryError("feishu_secret_unconfigured")
                secret = SecretStr(raw_secret)
            else:
                secret = self._settings.feishu_secret
            if self._client is None:
                with httpx.Client(
                    timeout=httpx.Timeout(10, connect=5), follow_redirects=False
                ) as client:
                    FeishuWebhook(url=webhook, secret=secret, client=client).send(
                        card, now=self._clock()
                    )
            else:
                FeishuWebhook(url=webhook, secret=secret, client=self._client).send(
                    card, now=self._clock()
                )
        except ValueError:
            error = FeishuDeliveryError("feishu_configuration_invalid")
            return self._record_failure(delivery_id, error)
        except FeishuDeliveryError as error:
            return self._record_failure(delivery_id, error)
        with self._sessions() as session:
            NotificationService(session, self._settings).finish(
                delivery_id=delivery_id,
                status=DeliveryStatus.SUCCEEDED,
                error_code=None,
                now=self._clock(),
            )
        return JobCompletion(status=JobStatus.SUCCEEDED)

    def _record_failure(self, delivery_id: UUID, error: FeishuDeliveryError) -> JobCompletion:
        now = self._clock()
        outcome = DeliveryStatus.UNKNOWN if error.uncertain else DeliveryStatus.FAILED
        with self._sessions() as session:
            NotificationService(session, self._settings).finish(
                delivery_id=delivery_id, status=outcome, error_code=error.code, now=now
            )
        if outcome is DeliveryStatus.FAILED:
            raise JobExecutionFailure(
                error_code=error.code,
                category=JobFailureCategory.TRANSIENT,
                occurred_at=now,
                next_action="检查飞书目标配置后重试",
                retry_at=now + timedelta(seconds=30),
                max_attempts=3,
            ) from None
        return JobCompletion(status=JobStatus.SUCCEEDED)
