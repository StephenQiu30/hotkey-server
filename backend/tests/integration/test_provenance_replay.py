from __future__ import annotations

import os
from collections.abc import Iterator
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest
from pydantic import ValidationError
from sqlalchemy import Engine, create_engine, text
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session, sessionmaker

from evidence.schemas import ProvenanceManifestInput, ProvenanceResourceRef
from evidence.services import (
    ProvenanceConflictError,
    ProvenanceService,
    ProvenanceUnavailableError,
)


@dataclass(frozen=True, slots=True)
class ProvenanceContext:
    engine: Engine
    sessions: sessionmaker[Session]
    owner_id: UUID
    job_id: UUID
    operation_id: UUID
    resources: tuple[UUID, UUID, UUID]
    now: datetime


@pytest.fixture
def provenance_context() -> Iterator[ProvenanceContext]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")

    engine = create_engine(database_url)
    sessions = sessionmaker(bind=engine, expire_on_commit=False)
    owner_id = uuid4()
    job_id = uuid4()
    operation_id = uuid4()
    policy_id = uuid4()
    retention_id = uuid4()
    resources = (uuid4(), uuid4(), uuid4())
    now = datetime(2026, 9, 21, 12, tzinfo=UTC)
    with engine.begin() as connection:
        connection.execute(
            text(
                "TRUNCATE content_discoveries, content_observations, content_records, "
                "source_capability_evidence, source_connection_versions, "
                "source_connections, provenance_manifest_inputs, provenance_manifests, "
                "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                "evidence_retention_policies, source_access_policies, "
                "resource_budget_reservations, resource_budget_windows, "
                "resource_budget_policies, resource_usage_attempts, "
                "resource_component_policies, job_stage_attempts, processed_messages, "
                "job_attempts, outbox_messages, jobs, monitor_topic_versions, monitor_topics, "
                "identity_sessions, identity_users"
            )
        )
        connection.execute(
            text(
                "INSERT INTO identity_users "
                "(id, username, password_hash, credential_version, created_at, updated_at) "
                "VALUES (:owner_id, 'provenance-owner', 'test-only-hash', 1, :now, :now)"
            ),
            {"owner_id": owner_id, "now": now},
        )
        connection.execute(
            text(
                "INSERT INTO source_access_policies "
                "(id, owner_id, source_key, capability, status, enabled, access_basis, "
                "terms_reference, processing_purpose, component_name, component_version, "
                "component_license, field_purposes, reviewed_at, review_expires_at, "
                "policy_version, created_at, updated_at) VALUES "
                "(:policy_id, :owner_id, 'manual', 'search', 'approved', true, "
                "'manual_import', 'test-fixture', 'provenance test', 'fixture', '1', "
                "'project-internal', '{\"external_id\": \"stable identity\"}'::jsonb, "
                ":now, :review_expires_at, 1, :now, :now)"
            ),
            {
                "owner_id": owner_id,
                "policy_id": policy_id,
                "now": now,
                "review_expires_at": now + timedelta(days=31),
            },
        )
        connection.execute(
            text(
                "INSERT INTO evidence_retention_policies "
                "(id, owner_id, source_policy_id, source_policy_version, data_class, "
                "requested_days, source_max_days, effective_days, policy_version, "
                "created_at, updated_at) VALUES "
                "(:retention_id, :owner_id, :policy_id, 1, 'structured', 30, 30, 30, 1, "
                ":now, :now)"
            ),
            {
                "owner_id": owner_id,
                "policy_id": policy_id,
                "retention_id": retention_id,
                "now": now,
            },
        )
        connection.execute(
            text(
                "INSERT INTO jobs "
                "(id, owner_id, operation_id, kind, configuration_ref, "
                "configuration_version, scope, request_fingerprint, created_at, updated_at) "
                "VALUES (:job_id, :owner_id, :operation_id, 'analysis.provenance', "
                "'test-config', 1, '{}'::jsonb, decode(repeat('00', 32), 'hex'), :now, :now)"
            ),
            {
                "owner_id": owner_id,
                "job_id": job_id,
                "operation_id": operation_id,
                "now": now,
            },
        )
        for resource in resources:
            connection.execute(
                text(
                    "INSERT INTO evidence_resources "
                    "(id, owner_id, resource_type, resource_id, source_policy_id, "
                    "source_policy_version, retention_policy_id, retention_policy_version, "
                    "data_class, collected_at, expires_at, cleanup_targets, created_at) "
                    "VALUES (:id, :owner_id, 'source_item', :resource_id, :policy_id, 1, "
                    ":retention_id, 1, 'structured', :now, :expires_at, '[]'::jsonb, :now)"
                ),
                {
                    "id": resource,
                    "owner_id": owner_id,
                    "resource_id": uuid4(),
                    "policy_id": policy_id,
                    "retention_id": retention_id,
                    "now": now,
                    "expires_at": now + timedelta(days=30),
                },
            )
    try:
        yield ProvenanceContext(
            engine=engine,
            sessions=sessions,
            owner_id=owner_id,
            job_id=job_id,
            operation_id=operation_id,
            resources=resources,
            now=now,
        )
    finally:
        with engine.begin() as connection:
            connection.execute(
                text(
                    "TRUNCATE content_discoveries, content_observations, content_records, "
                    "source_capability_evidence, source_connection_versions, "
                    "source_connections, provenance_manifest_inputs, provenance_manifests, "
                    "evidence_cleanup_targets, evidence_deletions, evidence_resources, "
                    "evidence_retention_policies, source_access_policies, "
                    "resource_budget_reservations, resource_budget_windows, "
                    "resource_budget_policies, resource_usage_attempts, "
                    "resource_component_policies, job_stage_attempts, processed_messages, "
                    "job_attempts, outbox_messages, jobs, monitor_topic_versions, monitor_topics, "
                    "identity_sessions, identity_users"
                )
            )
        engine.dispose()


def _command(
    context: ProvenanceContext,
    *,
    references: tuple[UUID, ...] | None = None,
    method_version: str = "1.0.0",
) -> ProvenanceManifestInput:
    reference_ids = references or context.resources[1:]
    return ProvenanceManifestInput(
        job_id=context.job_id,
        result_kind="controlled_score",
        method_key="fixture.weighted_score",
        method_version=method_version,
        method_parameters={"minimum_count": 3, "include_comments": True},
        subjects=(
            ProvenanceResourceRef(
                resource_record_id=context.resources[0], snapshot_ref="snapshot.subject.v1"
            ),
        ),
        references=tuple(
            ProvenanceResourceRef(
                resource_record_id=resource_id,
                snapshot_ref=f"snapshot.reference.{index}",
            )
            for index, resource_id in enumerate(reference_ids)
        ),
    )


def test_manifest_contract_rejects_secret_float_and_duplicate_inputs(
    provenance_context: ProvenanceContext,
) -> None:
    resource_id = provenance_context.resources[0]
    common = {
        "job_id": provenance_context.job_id,
        "result_kind": "controlled_score",
        "method_key": "fixture.weighted_score",
        "method_version": "1",
        "subjects": ({"resource_record_id": resource_id, "snapshot_ref": "snapshot.v1"},),
        "references": (
            {"resource_record_id": provenance_context.resources[1], "snapshot_ref": "ref.v1"},
        ),
    }
    with pytest.raises(ValidationError):
        ProvenanceManifestInput(**common, method_parameters={"weight": 0.5})
    with pytest.raises(ValidationError):
        ProvenanceManifestInput(**common, method_parameters={"access_token": "secret"})
    with pytest.raises(ValidationError):
        ProvenanceManifestInput(**common, method_parameters={"raw_query": "private terms"})
    duplicate = {**common, "references": common["subjects"]}
    with pytest.raises(ValidationError):
        ProvenanceManifestInput(**duplicate, method_parameters={})


def test_manifest_is_order_independent_persistent_and_idempotent(
    provenance_context: ProvenanceContext,
) -> None:
    canonical = _command(provenance_context)
    command = canonical.model_copy(update={"references": tuple(reversed(canonical.references))})
    with provenance_context.sessions() as session:
        service = ProvenanceService(session, clock=lambda: provenance_context.now)
        first = service.create(owner_id=provenance_context.owner_id, command=command)
        repeated = service.create(
            owner_id=provenance_context.owner_id,
            command=_command(provenance_context),
        )
        loaded = service.get(owner_id=provenance_context.owner_id, manifest_id=first.id)

    assert repeated == first
    assert loaded == first
    assert first.operation_id == provenance_context.operation_id
    assert len(first.manifest_fingerprint) == 64
    assert [item.ordinal for item in first.inputs if item.role == "reference"] == [0, 1]
    with provenance_context.engine.connect() as connection:
        assert (
            connection.execute(text("SELECT count(*) FROM provenance_manifests")).scalar_one() == 1
        )
        assert (
            connection.execute(text("SELECT count(*) FROM provenance_manifest_inputs")).scalar_one()
            == 3
        )
    with pytest.raises(IntegrityError), provenance_context.engine.begin() as connection:
        connection.execute(
            text("DELETE FROM evidence_resources WHERE id = :id"),
            {"id": provenance_context.resources[0]},
        )


def test_changed_reference_or_method_conflicts_with_frozen_result(
    provenance_context: ProvenanceContext,
) -> None:
    with provenance_context.sessions() as session:
        service = ProvenanceService(session, clock=lambda: provenance_context.now)
        service.create(owner_id=provenance_context.owner_id, command=_command(provenance_context))
        with pytest.raises(ProvenanceConflictError):
            service.create(
                owner_id=provenance_context.owner_id,
                command=_command(
                    provenance_context,
                    references=(provenance_context.resources[1],),
                ),
            )
        with pytest.raises(ProvenanceConflictError):
            service.create(
                owner_id=provenance_context.owner_id,
                command=_command(provenance_context, method_version="2.0.0"),
            )


def test_missing_or_deletion_blocked_resource_is_unavailable(
    provenance_context: ProvenanceContext,
) -> None:
    command = _command(provenance_context)
    missing = command.model_copy(
        update={
            "references": (
                ProvenanceResourceRef(resource_record_id=uuid4(), snapshot_ref="missing.v1"),
            )
        }
    )
    with provenance_context.sessions() as session:
        service = ProvenanceService(session, clock=lambda: provenance_context.now)
        with pytest.raises(ProvenanceUnavailableError):
            service.create(owner_id=provenance_context.owner_id, command=missing)
        created = service.create(owner_id=provenance_context.owner_id, command=command)

    with provenance_context.engine.begin() as connection:
        connection.execute(
            text(
                "INSERT INTO evidence_deletions "
                "(id, owner_id, operation_id, resource_record_id, reason, status, "
                "requested_at, cleanup_due_at, completed_at) VALUES "
                "(:id, :owner_id, :operation_id, :resource_id, 'user_request', "
                "'completed', :now, :due, :now)"
            ),
            {
                "id": uuid4(),
                "owner_id": provenance_context.owner_id,
                "operation_id": uuid4(),
                "resource_id": provenance_context.resources[0],
                "now": provenance_context.now,
                "due": provenance_context.now + timedelta(hours=24),
            },
        )
    with provenance_context.sessions() as session, pytest.raises(ProvenanceUnavailableError):
        ProvenanceService(session, clock=lambda: provenance_context.now).get(
            owner_id=provenance_context.owner_id,
            manifest_id=created.id,
        )
