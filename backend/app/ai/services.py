from __future__ import annotations

import hashlib
import json
import shlex
from collections.abc import Callable, Mapping, Sequence
from datetime import UTC, datetime
from typing import Any, Protocol
from uuid import UUID, uuid4

from sqlalchemy.engine import Connection
from sqlalchemy.orm import Session, sessionmaker

from ai.adapters.codex_app_server import CodexAppServerClient
from ai.models import AiCall
from ai.schemas import (
    AiCallError,
    AiCallStatus,
    AiCompletion,
    AiFailureCode,
    AiTokenUsage,
)
from core.config import Settings
from jobs.schemas import (
    BudgetContext,
    BudgetDecisionStatus,
    BudgetMetric,
    BudgetReservationInput,
    UsageAttemptInput,
    UsageKind,
    UsageOutcome,
)
from jobs.services import ResourceBudgetService

AI_COMPONENT_KEY = "codex.app-server"
_AI_USAGE_STAGE = "analysis.call"


class AiCompletionClient(Protocol):
    provider: str

    @property
    def model(self) -> str: ...

    def complete(
        self,
        *,
        prompt: str,
        output_schema: Mapping[str, Any],
        instructions: str = "",
    ) -> AiCompletion: ...

    def close(self) -> None: ...


def create_ai_client(settings: Settings) -> AiCompletionClient:
    command: Sequence[str] = shlex.split(settings.ai_command)
    return CodexAppServerClient(
        model=settings.ai_model,
        command=command,
        effort=settings.ai_effort,
        timeout_seconds=settings.ai_timeout_seconds,
    )


class AiService:
    def __init__(
        self,
        session: Session,
        client: AiCompletionClient,
        *,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        bind = session.get_bind()
        engine = bind.engine if isinstance(bind, Connection) else bind
        self._sessions = sessionmaker(engine, expire_on_commit=False)
        self._client = client
        self._clock = clock or (lambda: datetime.now(UTC))

    def complete(
        self,
        *,
        owner_id: UUID,
        job_id: UUID | None,
        purpose: str,
        prompt_version: str,
        prompt: str,
        output_schema: Mapping[str, Any],
        instructions: str = "",
    ) -> AiCompletion:
        if not 0 < len(purpose) <= 128:
            raise ValueError("purpose must contain at most 128 characters")
        if not 0 < len(prompt_version) <= 128:
            raise ValueError("prompt_version must contain at most 128 characters")
        if not 0 < len(self._client.provider) <= 64:
            raise ValueError("AI provider must contain at most 64 characters")
        if not 0 < len(self._client.model) <= 128:
            raise ValueError("AI model must contain at most 128 characters")
        call_id = uuid4()
        operation_id = job_id or call_id
        fingerprint = _input_fingerprint(
            model=self._client.model,
            prompt_version=prompt_version,
            prompt=prompt,
            output_schema=output_schema,
        )
        started_at = self._clock()
        self._reserve(
            owner_id=owner_id,
            job_id=job_id,
            operation_id=operation_id,
            attempt_id=call_id,
            started_at=started_at,
        )

        try:
            completion = self._client.complete(
                prompt=prompt,
                output_schema=output_schema,
                instructions=instructions,
            )
        except AiCallError as error:
            self._settle_and_record(
                call_id=call_id,
                owner_id=owner_id,
                job_id=job_id,
                purpose=purpose,
                prompt_version=prompt_version,
                fingerprint=fingerprint,
                status=AiCallStatus.FAILED,
                failure_code=error.code,
                usage=AiTokenUsage(),
                duration_ms=0,
            )
            raise
        except Exception as error:
            failure = AiCallError(AiFailureCode.FAILED, "AI client failed unexpectedly")
            self._settle_and_record(
                call_id=call_id,
                owner_id=owner_id,
                job_id=job_id,
                purpose=purpose,
                prompt_version=prompt_version,
                fingerprint=fingerprint,
                status=AiCallStatus.FAILED,
                failure_code=failure.code,
                usage=AiTokenUsage(),
                duration_ms=0,
            )
            raise failure from error

        self._settle_and_record(
            call_id=call_id,
            owner_id=owner_id,
            job_id=job_id,
            purpose=purpose,
            prompt_version=prompt_version,
            fingerprint=fingerprint,
            status=AiCallStatus.SUCCEEDED,
            failure_code=None,
            usage=completion.usage,
            duration_ms=completion.duration_ms,
        )
        return completion.model_copy(update={"call_id": call_id})

    def _reserve(
        self,
        *,
        owner_id: UUID,
        job_id: UUID | None,
        operation_id: UUID,
        attempt_id: UUID,
        started_at: datetime,
    ) -> None:
        with self._sessions() as session:
            budget = ResourceBudgetService(session, clock=self._clock)
            with session.begin():
                decision = budget.reserve_budget_in_transaction(
                    owner_id=owner_id,
                    command=BudgetReservationInput(
                        reservation_id=attempt_id,
                        operation_id=operation_id,
                        metric=BudgetMetric.ANALYSIS_ATTEMPT,
                        requested_units=1,
                        context=BudgetContext(
                            job_ref=f"job:{job_id.hex}" if job_id is not None else None
                        ),
                    ),
                )
                if decision.status is BudgetDecisionStatus.DELAYED:
                    raise AiCallError(AiFailureCode.RATE_LIMITED, "analysis budget is exhausted")
                attempt = budget.begin_attempt_in_transaction(
                    owner_id=owner_id,
                    command=UsageAttemptInput(
                        attempt_id=attempt_id,
                        operation_id=operation_id,
                        component_key=AI_COMPONENT_KEY,
                        usage_kind=UsageKind.ANALYSIS_ATTEMPT,
                        stage=_AI_USAGE_STAGE,
                        started_at=started_at,
                    ),
                )
                if attempt.outcome is not UsageOutcome.STARTED:
                    raise AiCallError(AiFailureCode.FAILED, "analysis attempt is already settled")

    def _settle_and_record(
        self,
        *,
        call_id: UUID,
        owner_id: UUID,
        job_id: UUID | None,
        purpose: str,
        prompt_version: str,
        fingerprint: bytes,
        status: AiCallStatus,
        failure_code: AiFailureCode | None,
        usage: AiTokenUsage,
        duration_ms: int,
    ) -> None:
        """Settle usage and persist the audit row in a service-owned transaction."""
        finished_at = self._clock()
        with self._sessions() as session:
            budget = ResourceBudgetService(session, clock=self._clock)
            with session.begin():
                budget.settle_budget_reservation_in_transaction(
                    owner_id=owner_id,
                    reservation_id=call_id,
                    actual_units=1,
                )
                budget.finish_attempt_in_transaction(
                    owner_id=owner_id,
                    attempt_id=call_id,
                    outcome=(
                        UsageOutcome.SUCCEEDED
                        if status is AiCallStatus.SUCCEEDED
                        else UsageOutcome.FAILED
                    ),
                    finished_at=finished_at,
                )
                session.add(
                    AiCall(
                        id=call_id,
                        owner_id=owner_id,
                        job_id=job_id,
                        purpose=purpose,
                        provider=self._client.provider,
                        model=self._client.model,
                        prompt_version=prompt_version,
                        input_fingerprint=fingerprint,
                        status=status.value,
                        failure_code=failure_code.value if failure_code is not None else None,
                        input_tokens=usage.input_tokens,
                        cached_input_tokens=usage.cached_input_tokens,
                        output_tokens=usage.output_tokens,
                        reasoning_output_tokens=usage.reasoning_output_tokens,
                        duration_ms=duration_ms,
                        created_at=finished_at,
                    )
                )


def _input_fingerprint(
    *,
    model: str,
    prompt_version: str,
    prompt: str,
    output_schema: Mapping[str, Any],
) -> bytes:
    normalized_schema = json.dumps(
        output_schema,
        ensure_ascii=False,
        allow_nan=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    digest = hashlib.sha256()
    for value in (model, prompt_version, prompt, normalized_schema):
        digest.update(value.encode())
    return digest.digest()
