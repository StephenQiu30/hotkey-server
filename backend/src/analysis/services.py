import json
from collections import Counter, defaultdict, deque
from datetime import UTC, datetime
from hashlib import sha256
from uuid import UUID, uuid4

from sqlalchemy import func, select
from sqlalchemy.orm import Session, sessionmaker

from analysis.models import AnalysisLabel, AnalysisRun, AnalysisSample
from analysis.schemas import (
    AnalysisComposition,
    AnalysisContextView,
    AnalysisLabelInput,
    AnalysisLabelView,
    AnalysisRunInput,
    AnalysisRunPage,
    AnalysisRunSummary,
    AnalysisRunView,
    AnalysisSampleView,
    AnalysisViewpoint,
    ControlledCommentStatistics,
)
from audit.services import audit
from contents.services import (
    AnalysisTextVersion,
    ContentAnalysisCandidate,
    content_analysis_candidates,
    content_version_contexts,
    current_content_version_ids,
)
from core.clock import utcnow
from core.errors import AppError
from events.services import event_analysis_scope

SAMPLING_POLICY_VERSION = "event-comment-stratified-v1"
LABEL_SCHEMA_VERSION = "analysis-label-v1"


def _digest(value: object) -> str:
    payload = json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    return sha256(payload.encode()).hexdigest()


def _time_bucket(value: datetime) -> datetime:
    value = value.astimezone(UTC)
    return value.replace(hour=0, minute=0, second=0, microsecond=0)


def _select_candidates(
    candidates: list[ContentAnalysisCandidate], max_items: int
) -> list[ContentAnalysisCandidate]:
    strata: dict[tuple[str, datetime, str, str], list[ContentAnalysisCandidate]] = defaultdict(list)
    for candidate in candidates:
        key = (
            candidate.source,
            _time_bucket(candidate.published_at),
            candidate.root_external_id,
            "provider_default",
        )
        strata[key].append(candidate)
    queues: list[deque[ContentAnalysisCandidate]] = []
    for key in sorted(strata):
        ordered = sorted(
            strata[key],
            key=lambda item: _digest([SAMPLING_POLICY_VERSION, str(item.content_version_id)]),
        )
        queues.append(deque(ordered))
    selected: list[ContentAnalysisCandidate] = []
    while queues and len(selected) < max_items:
        remaining: list[deque[ContentAnalysisCandidate]] = []
        for queue in queues:
            if len(selected) == max_items:
                break
            selected.append(queue.popleft())
            if queue:
                remaining.append(queue)
        queues = remaining
    return selected


def _sample_manifest(sample: AnalysisSample) -> dict[str, object]:
    return {
        "position": sample.position,
        "content_id": str(sample.content_id),
        "content_version_id": str(sample.content_version_id),
        "parent_content_version_id": (
            str(sample.parent_content_version_id)
            if sample.parent_content_version_id is not None
            else None
        ),
        "root_content_version_id": (
            str(sample.root_content_version_id)
            if sample.root_content_version_id is not None
            else None
        ),
        "source": sample.source,
        "kind": sample.kind,
        "root_external_id": sample.root_external_id,
        "published_at": sample.published_at.isoformat(),
        "time_bucket_start": sample.time_bucket_start.isoformat(),
        "ordering_origin": sample.ordering_origin,
        "selection_reason": sample.selection_reason,
        "text_sha256": sample.text_sha256,
        "parent_text_sha256": sample.parent_text_sha256,
        "root_text_sha256": sample.root_text_sha256,
    }


class AnalysisService:
    def __init__(self, factory: sessionmaker[Session]):
        self.factory = factory

    @staticmethod
    def _run(session: Session, identity: UUID) -> AnalysisRun:
        run = session.get(AnalysisRun, identity)
        if run is None:
            raise AppError("analysis_run_not_found", 404)
        return run

    @staticmethod
    def _samples(session: Session, run_id: UUID) -> list[AnalysisSample]:
        return list(
            session.scalars(
                select(AnalysisSample)
                .where(AnalysisSample.run_id == run_id)
                .order_by(AnalysisSample.position)
            )
        )

    @staticmethod
    def _labels(session: Session, sample_ids: list[UUID]) -> dict[UUID, AnalysisLabel]:
        if not sample_ids:
            return {}
        return {
            label.sample_id: label
            for label in session.scalars(
                select(AnalysisLabel).where(AnalysisLabel.sample_id.in_(sample_ids))
            )
        }

    @staticmethod
    def _label_view(label: AnalysisLabel) -> AnalysisLabelView:
        return AnalysisLabelView.model_validate(
            {
                "id": label.id,
                "topic": label.topic,
                "target": label.target,
                "sentiment": label.sentiment,
                "stance": label.stance,
                "request": label.request,
                "abstained": label.abstained,
                "citation_content_version_ids": [
                    UUID(identity) for identity in label.citation_content_version_ids
                ],
                "label_source": label.label_source,
                "schema_version": label.schema_version,
                "created_at": label.created_at,
            }
        )

    @staticmethod
    def _context_view(
        role: str,
        identity: UUID,
        expected_hash: str,
        contexts: dict[UUID, AnalysisTextVersion],
    ) -> AnalysisContextView:
        context = contexts.get(identity)
        available = (
            context is not None
            and context.visibility == "available"
            and context.text_sha256 == expected_hash
        )
        text = context.text if context is not None and available else None
        canonical_url = context.canonical_url if context is not None and available else None
        return AnalysisContextView.model_validate(
            {
                "role": role,
                "content_id": context.content_id if context is not None else UUID(int=0),
                "content_version_id": identity,
                "text": text,
                "text_sha256": expected_hash,
                "canonical_url": canonical_url,
                "available": available,
            }
        )

    @classmethod
    def _view(
        cls, session: Session, run: AnalysisRun, *, recomputed: bool = False
    ) -> AnalysisRunView:
        samples = cls._samples(session, run.id)
        labels = cls._labels(session, [sample.id for sample in samples])
        version_ids = {
            identity
            for sample in samples
            for identity in (
                sample.content_version_id,
                sample.parent_content_version_id,
                sample.root_content_version_id,
            )
            if identity is not None
        }
        contexts = content_version_contexts(session, version_ids)
        current = current_content_version_ids(
            session, {context.content_id for context in contexts.values()}
        )
        scope = event_analysis_scope(session, run.event_id)
        stale = scope.current_revision != run.event_revision or len(contexts) != len(version_ids)
        stale = stale or any(
            context.visibility != "available"
            or current.get(context.content_id) != context.content_version_id
            for context in contexts.values()
        )

        sample_views: list[AnalysisSampleView] = []
        context_by_id: dict[UUID, AnalysisContextView] = {}
        for sample in samples:
            frozen_contexts = [
                cls._context_view("sample", sample.content_version_id, sample.text_sha256, contexts)
            ]
            if sample.parent_content_version_id is not None:
                assert sample.parent_text_sha256 is not None
                frozen_contexts.append(
                    cls._context_view(
                        "parent",
                        sample.parent_content_version_id,
                        sample.parent_text_sha256,
                        contexts,
                    )
                )
            if (
                sample.root_content_version_id is not None
                and sample.root_content_version_id
                not in {context.content_version_id for context in frozen_contexts}
            ):
                assert sample.root_text_sha256 is not None
                frozen_contexts.append(
                    cls._context_view(
                        "root",
                        sample.root_content_version_id,
                        sample.root_text_sha256,
                        contexts,
                    )
                )
            for context in frozen_contexts:
                context_by_id.setdefault(context.content_version_id, context)
            sample_views.append(
                AnalysisSampleView.model_validate(
                    {
                        "id": sample.id,
                        "position": sample.position,
                        "source": sample.source,
                        "kind": sample.kind,
                        "root_external_id": sample.root_external_id,
                        "published_at": sample.published_at,
                        "time_bucket_start": sample.time_bucket_start,
                        "ordering_origin": sample.ordering_origin,
                        "selection_reason": sample.selection_reason,
                        "contexts": frozen_contexts,
                        "label": (
                            cls._label_view(labels[sample.id]) if sample.id in labels else None
                        ),
                    }
                )
            )

        viewpoint_counts: Counter[tuple[str, str, str, str, str]] = Counter()
        viewpoint_citations: dict[tuple[str, str, str, str, str], set[UUID]] = defaultdict(set)
        for label in labels.values():
            if label.abstained:
                continue
            key = (label.topic, label.target, label.sentiment, label.stance, label.request)
            viewpoint_counts[key] += 1
            viewpoint_citations[key].update(
                UUID(identity) for identity in label.citation_content_version_ids
            )
        viewpoints = [
            AnalysisViewpoint.model_validate(
                {
                    "topic": key[0],
                    "target": key[1],
                    "sentiment": key[2],
                    "stance": key[3],
                    "request": key[4],
                    "sample_count": viewpoint_counts[key],
                    "citations": [
                        context_by_id[identity]
                        for identity in sorted(viewpoint_citations[key])
                        if identity in context_by_id and context_by_id[identity].available
                    ],
                }
            )
            for key in sorted(viewpoint_counts, key=lambda item: (-viewpoint_counts[item], item))
            if any(
                identity in context_by_id and context_by_id[identity].available
                for identity in viewpoint_citations[key]
            )
        ]
        platform_counts = Counter(sample.source for sample in samples)
        bucket_counts = Counter(sample.time_bucket_start.isoformat() for sample in samples)
        ordering_counts = Counter(sample.ordering_origin for sample in samples)
        abstained = sum(label.abstained for label in labels.values())
        status = "stale" if stale else run.state
        return AnalysisRunView.model_validate(
            {
                "id": run.id,
                "event_id": run.event_id,
                "event_revision": run.event_revision,
                "status": status,
                "method": run.method,
                "analyzer_id": run.analyzer_id,
                "prompt_version": run.prompt_version,
                "label_schema_version": run.label_schema_version,
                "sampling_policy_version": run.sampling_policy_version,
                "manifest_sha256": run.manifest_sha256,
                "since": run.since_at,
                "until": run.until_at,
                "cutoff": run.cutoff_at,
                "max_items": run.max_items,
                "sample_count": run.sample_count,
                "labeled_count": len(labels),
                "valid_labeled_count": len(labels) - abstained,
                "abstained_count": abstained,
                "pending_count": run.sample_count - len(labels),
                "token_budget": run.token_budget,
                "input_tokens": run.input_tokens,
                "output_tokens": run.output_tokens,
                "composition": AnalysisComposition(
                    platforms=dict(sorted(platform_counts.items())),
                    roots=len({(sample.source, sample.root_external_id) for sample in samples}),
                    time_buckets=dict(sorted(bucket_counts.items())),
                    ordering_origins=dict(sorted(ordering_counts.items())),
                ),
                "samples": sample_views,
                "viewpoints": viewpoints,
                "created_at": run.created_at,
                "updated_at": run.updated_at,
                "recomputed_from_manifest": recomputed,
            }
        )

    def create(self, event_id: UUID, data: AnalysisRunInput) -> AnalysisRunView:
        request_hash = _digest(
            {
                "event_id": str(event_id),
                "event_revision": data.expected_event_revision,
                "since": data.since.isoformat(),
                "until": data.until.isoformat(),
                "cutoff": data.cutoff.isoformat(),
                "max_items": data.max_items,
                "sampling_policy_version": SAMPLING_POLICY_VERSION,
            }
        )
        with self.factory.begin() as session:
            scope = event_analysis_scope(session, event_id, lock=True)
            if scope.current_revision != data.expected_event_revision:
                raise AppError("version_conflict", 409)
            existing = session.scalar(
                select(AnalysisRun).where(
                    AnalysisRun.event_id == event_id,
                    AnalysisRun.request_sha256 == request_hash,
                )
            )
            if existing is not None:
                return self._view(session, existing)
            candidates = content_analysis_candidates(
                session,
                scope.member_content_ids,
                data.since,
                data.until,
                data.cutoff,
            )
            selected = _select_candidates(candidates, data.max_items)
            if not selected:
                raise AppError("analysis_sample_empty", 409)
            now = utcnow()
            run = AnalysisRun(
                id=uuid4(),
                event_id=event_id,
                event_revision=scope.current_revision,
                state="pending",
                method="manual_baseline",
                analyzer_id="manual",
                prompt_version="none",
                label_schema_version=LABEL_SCHEMA_VERSION,
                sampling_policy_version=SAMPLING_POLICY_VERSION,
                request_sha256=request_hash,
                manifest_sha256="0" * 64,
                since_at=data.since,
                until_at=data.until,
                cutoff_at=data.cutoff,
                max_items=data.max_items,
                sample_count=len(selected),
                token_budget=0,
                input_tokens=0,
                output_tokens=0,
                created_at=now,
                updated_at=now,
            )
            session.add(run)
            samples: list[AnalysisSample] = []
            for position, candidate in enumerate(selected, start=1):
                sample = AnalysisSample(
                    id=uuid4(),
                    run_id=run.id,
                    position=position,
                    content_id=candidate.content_id,
                    content_version_id=candidate.content_version_id,
                    parent_content_version_id=(
                        candidate.parent_context.content_version_id
                        if candidate.parent_context is not None
                        else None
                    ),
                    root_content_version_id=(
                        candidate.root_context.content_version_id
                        if candidate.root_context is not None
                        else None
                    ),
                    source=candidate.source,
                    kind=candidate.kind,
                    root_external_id=candidate.root_external_id,
                    published_at=candidate.published_at,
                    time_bucket_start=_time_bucket(candidate.published_at),
                    ordering_origin="provider_default",
                    selection_reason="stratum_round_robin",
                    text_sha256=candidate.text_sha256,
                    parent_text_sha256=(
                        candidate.parent_context.text_sha256
                        if candidate.parent_context is not None
                        else None
                    ),
                    root_text_sha256=(
                        candidate.root_context.text_sha256
                        if candidate.root_context is not None
                        else None
                    ),
                )
                session.add(sample)
                samples.append(sample)
            session.flush()
            run.manifest_sha256 = _digest(
                {
                    "event_id": str(run.event_id),
                    "event_revision": run.event_revision,
                    "since": run.since_at.isoformat(),
                    "until": run.until_at.isoformat(),
                    "cutoff": run.cutoff_at.isoformat(),
                    "sampling_policy_version": run.sampling_policy_version,
                    "samples": [_sample_manifest(sample) for sample in samples],
                }
            )
            audit(session, "analysis_sample_frozen", f"{event_id}:{run.id}")
            return self._view(session, run)

    def list_for_event(self, event_id: UUID, limit: int) -> AnalysisRunPage:
        with self.factory() as session:
            event_analysis_scope(session, event_id)
            rows = list(
                session.scalars(
                    select(AnalysisRun)
                    .where(AnalysisRun.event_id == event_id)
                    .order_by(AnalysisRun.created_at.desc(), AnalysisRun.id.desc())
                    .limit(limit)
                )
            )
            views = [self._view(session, row) for row in rows]
            return AnalysisRunPage(
                items=[
                    AnalysisRunSummary(
                        id=view.id,
                        event_id=view.event_id,
                        event_revision=view.event_revision,
                        status=view.status,
                        manifest_sha256=view.manifest_sha256,
                        sample_count=view.sample_count,
                        labeled_count=view.labeled_count,
                        abstained_count=view.abstained_count,
                        created_at=view.created_at,
                    )
                    for view in views
                ]
            )

    def get(self, identity: UUID) -> AnalysisRunView:
        with self.factory() as session:
            return self._view(session, self._run(session, identity))

    def label(
        self,
        identity: UUID,
        sample_id: UUID,
        data: AnalysisLabelInput,
    ) -> AnalysisRunView:
        with self.factory.begin() as session:
            run = self._run(session, identity)
            sample = session.scalar(
                select(AnalysisSample)
                .where(
                    AnalysisSample.id == sample_id,
                    AnalysisSample.run_id == run.id,
                )
                .with_for_update()
            )
            if sample is None:
                raise AppError("analysis_sample_not_found", 404)
            allowed = {
                candidate
                for candidate in (
                    sample.content_version_id,
                    sample.parent_content_version_id,
                    sample.root_content_version_id,
                )
                if candidate is not None
            }
            if not set(data.citation_content_version_ids).issubset(allowed):
                raise AppError("analysis_citation_outside_sample", 422)
            existing = session.scalar(
                select(AnalysisLabel).where(AnalysisLabel.sample_id == sample.id)
            )
            values = {
                "topic": data.topic,
                "target": data.target,
                "sentiment": data.sentiment,
                "stance": data.stance,
                "request": data.request,
                "abstained": data.abstained,
                "citation_content_version_ids": [
                    str(value) for value in data.citation_content_version_ids
                ],
            }
            if existing is not None:
                current = {key: getattr(existing, key) for key in values}
                if current != values:
                    raise AppError("analysis_label_exists", 409)
                return self._view(session, run)
            now = utcnow()
            session.add(
                AnalysisLabel(
                    id=uuid4(),
                    sample_id=sample.id,
                    **values,
                    label_source="manual",
                    schema_version=LABEL_SCHEMA_VERSION,
                    created_at=now,
                )
            )
            session.flush()
            labeled_count = session.scalar(
                select(func.count(AnalysisLabel.id))
                .join(AnalysisSample, AnalysisSample.id == AnalysisLabel.sample_id)
                .where(AnalysisSample.run_id == run.id)
            )
            if labeled_count == run.sample_count:
                run.state = "succeeded"
            run.updated_at = now
            audit(session, "analysis_sample_labeled", f"{run.id}:{sample.id}")
            return self._view(session, run)

    def recompute(self, identity: UUID) -> AnalysisRunView:
        with self.factory() as session:
            run = self._run(session, identity)
            samples = self._samples(session, run.id)
            manifest = _digest(
                {
                    "event_id": str(run.event_id),
                    "event_revision": run.event_revision,
                    "since": run.since_at.isoformat(),
                    "until": run.until_at.isoformat(),
                    "cutoff": run.cutoff_at.isoformat(),
                    "sampling_policy_version": run.sampling_policy_version,
                    "samples": [_sample_manifest(sample) for sample in samples],
                }
            )
            if manifest != run.manifest_sha256:
                raise AppError("analysis_manifest_corrupt", 500)
            return self._view(session, run, recomputed=True)


def analysis_knowledge_snapshot(
    session: Session, identity: UUID, *, lock: bool = False
) -> AnalysisRunView:
    query = select(AnalysisRun).where(AnalysisRun.id == identity)
    if lock:
        query = query.with_for_update()
    run = session.scalar(query)
    if run is None:
        raise AppError("analysis_run_not_found", 404)
    return AnalysisService._view(session, run)


def controlled_comment_statistics(
    session: Session,
    event_id: UUID,
    since: datetime,
    until: datetime,
) -> ControlledCommentStatistics:
    scope = event_analysis_scope(session, event_id)
    candidates = content_analysis_candidates(
        session,
        scope.member_content_ids,
        since,
        until,
        until,
    )
    unique = {candidate.content_id: candidate for candidate in candidates}
    return ControlledCommentStatistics(
        event_id=scope.id,
        event_revision=scope.current_revision,
        since=since,
        until=until,
        total=len(unique),
        by_source=dict(sorted(Counter(item.source for item in unique.values()).items())),
        by_kind=dict(sorted(Counter(item.kind for item in unique.values()).items())),
    )
