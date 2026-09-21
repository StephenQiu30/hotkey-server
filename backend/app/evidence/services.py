from __future__ import annotations

import math
from collections.abc import Callable, Mapping
from copy import deepcopy
from datetime import UTC, datetime
from typing import TypeGuard, cast
from uuid import UUID, uuid4

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session

from evidence.models import SourceAccessPolicy
from evidence.schemas import (
    AccessBasis,
    AccessPolicyStatus,
    AdmittedScalar,
    AdmittedSourcePayload,
    AdmittedValue,
    SourceAccessPolicyInput,
    SourceAccessPolicyView,
    SourceCapability,
)

type Clock = Callable[[], datetime]


class SourceAccessUnavailableError(RuntimeError):
    """The requested source capability is not currently approved and enabled."""


def _is_admitted_scalar(value: object) -> TypeGuard[AdmittedScalar]:
    return value is None or isinstance(value, (str, int, float, bool))


def _copy_admitted_value(value: object) -> AdmittedValue:
    if _is_admitted_scalar(value):
        if isinstance(value, float) and not math.isfinite(value):
            raise ValueError("admitted numeric fields must be finite")
        return value
    if isinstance(value, list) and all(_is_admitted_scalar(item) for item in value):
        scalar_values = cast(list[AdmittedScalar], value)
        if any(isinstance(item, float) and not math.isfinite(item) for item in scalar_values):
            raise ValueError("admitted numeric fields must be finite")
        return deepcopy(scalar_values)
    raise ValueError("admitted fields must be JSON scalars or scalar lists")


def minimize_payload(
    *,
    field_purposes: Mapping[str, str],
    payload: Mapping[str, object],
) -> dict[str, AdmittedValue]:
    minimized: dict[str, AdmittedValue] = {}
    for field_name in field_purposes:
        if field_name in payload:
            minimized[field_name] = _copy_admitted_value(payload[field_name])
    return minimized


class SourceAccessPolicyService:
    def __init__(self, session: Session, *, clock: Clock | None = None) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def save(
        self,
        *,
        owner_id: UUID,
        command: SourceAccessPolicyInput,
    ) -> SourceAccessPolicyView:
        now = self._clock()
        if command.enabled and (
            command.reviewed_at is None
            or command.reviewed_at > now
            or (command.review_expires_at is not None and command.review_expires_at <= now)
        ):
            raise ValueError("enabled policy review must be current")

        values = {
            "status": command.status.value,
            "enabled": command.enabled,
            "access_basis": command.access_basis.value if command.access_basis else None,
            "terms_reference": command.terms_reference,
            "processing_purpose": command.processing_purpose,
            "component_name": command.component_name,
            "component_version": command.component_version,
            "component_license": command.component_license,
            "field_purposes": command.field_purposes,
            "reviewed_at": command.reviewed_at,
            "review_expires_at": command.review_expires_at,
            "updated_at": now,
        }
        statement = (
            insert(SourceAccessPolicy)
            .values(
                id=uuid4(),
                owner_id=owner_id,
                source_key=command.source_key,
                capability=command.capability.value,
                policy_version=1,
                created_at=now,
                **values,
            )
            .on_conflict_do_update(
                constraint="source_access_policies_owner_source_capability_key",
                set_={
                    **values,
                    "policy_version": SourceAccessPolicy.policy_version + 1,
                },
            )
            .returning(SourceAccessPolicy)
        )
        self._session.rollback()
        with self._session.begin():
            model = self._session.scalar(statement)
            if model is None:
                raise RuntimeError("source access policy was not returned")
            view = self._view(model)
        return view

    def get(
        self,
        *,
        owner_id: UUID,
        source_key: str,
        capability: SourceCapability,
    ) -> SourceAccessPolicyView:
        model = self._find(owner_id, source_key, capability)
        if model is None:
            self._session.rollback()
            raise SourceAccessUnavailableError("source access policy is unavailable")
        try:
            return self._view(model)
        finally:
            self._session.rollback()

    def admit_payload(
        self,
        *,
        owner_id: UUID,
        source_key: str,
        capability: SourceCapability,
        payload: Mapping[str, object],
    ) -> AdmittedSourcePayload:
        model = self._find(owner_id, source_key, capability)
        now = self._clock()
        if (
            model is None
            or model.status != AccessPolicyStatus.APPROVED.value
            or not model.enabled
            or not model.field_purposes
            or (model.review_expires_at is not None and model.review_expires_at <= now)
        ):
            self._session.rollback()
            raise SourceAccessUnavailableError("source access policy is unavailable")
        policy_id = model.id
        policy_version = model.policy_version
        policy_owner_id = model.owner_id
        policy_source_key = model.source_key
        policy_capability = SourceCapability(model.capability)
        field_purposes = dict(model.field_purposes)
        self._session.rollback()
        fields = minimize_payload(
            field_purposes=field_purposes,
            payload=payload,
        )
        if not fields:
            raise ValueError("payload contains no admitted fields")
        admitted = AdmittedSourcePayload(
            policy_id=policy_id,
            policy_version=policy_version,
            owner_id=policy_owner_id,
            source_key=policy_source_key,
            capability=policy_capability,
            fields=fields,
        )
        return admitted

    def _find(
        self,
        owner_id: UUID,
        source_key: str,
        capability: SourceCapability,
    ) -> SourceAccessPolicy | None:
        return self._session.scalar(
            select(SourceAccessPolicy).where(
                SourceAccessPolicy.owner_id == owner_id,
                SourceAccessPolicy.source_key == source_key,
                SourceAccessPolicy.capability == capability.value,
            )
        )

    @staticmethod
    def _view(model: SourceAccessPolicy) -> SourceAccessPolicyView:
        return SourceAccessPolicyView(
            id=model.id,
            owner_id=model.owner_id,
            source_key=model.source_key,
            capability=SourceCapability(model.capability),
            status=AccessPolicyStatus(model.status),
            enabled=model.enabled,
            access_basis=(AccessBasis(model.access_basis) if model.access_basis else None),
            terms_reference=model.terms_reference,
            processing_purpose=model.processing_purpose,
            component_name=model.component_name,
            component_version=model.component_version,
            component_license=model.component_license,
            field_purposes=dict(model.field_purposes),
            reviewed_at=model.reviewed_at,
            review_expires_at=model.review_expires_at,
            policy_version=model.policy_version,
            created_at=model.created_at,
            updated_at=model.updated_at,
        )
