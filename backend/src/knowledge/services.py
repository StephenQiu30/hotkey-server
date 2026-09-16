import json
import re
import unicodedata
from hashlib import sha256
from typing import Literal
from uuid import UUID, uuid4

from sqlalchemy import select
from sqlalchemy.orm import Session, sessionmaker

from ai.contracts import EmbeddingProvider, EmbeddingProviderError
from analysis.schemas import AnalysisRunView
from analysis.services import analysis_knowledge_snapshot, controlled_comment_statistics
from audit.services import audit
from contents.schemas import (
    ContentWithdrawalInput,
    ContentWithdrawalManifest,
    ContentWithdrawalReplayView,
    ContentWithdrawalView,
)
from contents.services import (
    apply_withdrawal_manifest_entry,
    content_version_contexts,
    current_content_version_ids,
    export_withdrawal_manifest,
    withdraw_content,
)
from core.clock import utcnow
from core.errors import AppError
from events.services import event_title_for_knowledge
from evidence.services import schedule_raw_page_deletions
from knowledge.models import KnowledgeChunk, KnowledgeCitation, KnowledgeEntry, KnowledgeVersion
from knowledge.schemas import (
    CommentCountQuestion,
    EvidenceQuestion,
    KnowledgeAnswer,
    KnowledgeCitationView,
    KnowledgeEntryView,
    KnowledgePage,
    KnowledgeQuestion,
)


def normalize_search_text(value: str) -> str:
    normalized = unicodedata.normalize("NFKC", value).casefold()
    return re.sub(r"\s+", " ", normalized).strip()


def _digest(value: object) -> str:
    payload = json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    return sha256(payload.encode()).hexdigest()


def _snapshot_body(snapshot: AnalysisRunView) -> str:
    lines = [
        (
            f"范围：{snapshot.since.isoformat()} 至 {snapshot.until.isoformat()}；"
            f"固定样本 {snapshot.sample_count} 条，已标注 {snapshot.labeled_count} 条，"
            f"弃判 {snapshot.abstained_count} 条。"
        )
    ]
    for position, viewpoint in enumerate(snapshot.viewpoints, start=1):
        line = (
            f"观点 {position}：主题 {viewpoint.topic}；对象 {viewpoint.target}；"
            f"情绪 {viewpoint.sentiment}；立场 {viewpoint.stance}；"
            f"本次样本 {viewpoint.sample_count} 条。"
        )
        if viewpoint.request:
            line += f"诉求：{viewpoint.request}。"
        lines.append(line)
    return "\n".join(lines)


class KnowledgeService:
    def __init__(
        self,
        factory: sessionmaker[Session],
        embedding_provider: EmbeddingProvider | None = None,
    ):
        self.factory = factory
        self.embedding_provider = embedding_provider

    @staticmethod
    def _entry(session: Session, identity: UUID) -> KnowledgeEntry:
        entry = session.get(KnowledgeEntry, identity)
        if entry is None:
            raise AppError("knowledge_entry_not_found", 404)
        return entry

    @staticmethod
    def _version(session: Session, entry: KnowledgeEntry) -> KnowledgeVersion:
        version = session.scalar(
            select(KnowledgeVersion).where(
                KnowledgeVersion.entry_id == entry.id,
                KnowledgeVersion.version == entry.current_version,
            )
        )
        if version is None:
            raise AppError("knowledge_version_missing", 500)
        return version

    @classmethod
    def _view(
        cls,
        session: Session,
        entry: KnowledgeEntry,
        *,
        similarity: float | None = None,
    ) -> KnowledgeEntryView:
        version = cls._version(session, entry)
        chunk = session.scalar(
            select(KnowledgeChunk).where(KnowledgeChunk.version_id == version.id)
        )
        if chunk is None:
            raise AppError("knowledge_chunk_missing", 500)
        citation_rows = list(
            session.scalars(
                select(KnowledgeCitation)
                .where(KnowledgeCitation.version_id == version.id)
                .order_by(KnowledgeCitation.position)
            )
        )
        version_ids = {citation.content_version_id for citation in citation_rows}
        contexts = content_version_contexts(session, version_ids)
        current = current_content_version_ids(
            session, {context.content_id for context in contexts.values()}
        )
        citation_views: list[KnowledgeCitationView] = []
        for citation in citation_rows:
            context = contexts.get(citation.content_version_id)
            available = (
                context is not None
                and context.visibility == "available"
                and context.text_sha256 == citation.text_sha256
                and current.get(context.content_id) == context.content_version_id
            )
            citation_views.append(
                KnowledgeCitationView(
                    content_version_id=citation.content_version_id,
                    text_sha256=citation.text_sha256,
                    text=context.text if available and context is not None else None,
                    canonical_url=(
                        context.canonical_url if available and context is not None else None
                    ),
                    available=available,
                )
            )
        snapshot = analysis_knowledge_snapshot(session, entry.source_analysis_run_id)
        stale = (
            snapshot.status != "succeeded"
            or snapshot.manifest_sha256 != version.analysis_manifest_sha256
            or not citation_views
            or any(not citation.available for citation in citation_views)
        )
        return KnowledgeEntryView(
            id=entry.id,
            event_id=entry.event_id,
            source_analysis_run_id=entry.source_analysis_run_id,
            entry_type="analysis_snapshot",
            version=version.version,
            title=version.title,
            body=version.body,
            analysis_manifest_sha256=version.analysis_manifest_sha256,
            semantic_index_state=chunk.index_state,
            similarity=similarity,
            stale=stale,
            citations=citation_views,
            created_at=entry.created_at,
            updated_at=entry.updated_at,
        )

    def publish_analysis(self, analysis_run_id: UUID) -> KnowledgeEntryView:
        with self.factory.begin() as session:
            existing = session.scalar(
                select(KnowledgeEntry).where(
                    KnowledgeEntry.source_analysis_run_id == analysis_run_id
                )
            )
            if existing is not None:
                return self._view(session, existing)
            snapshot = analysis_knowledge_snapshot(session, analysis_run_id, lock=True)
            if snapshot.status == "stale":
                raise AppError("analysis_stale", 409)
            if snapshot.status != "succeeded":
                raise AppError("analysis_not_ready", 409)
            if not snapshot.viewpoints:
                raise AppError("analysis_has_no_supported_viewpoint", 409)
            title = f"{event_title_for_knowledge(session, snapshot.event_id)} · 评论分析"
            body = _snapshot_body(snapshot)
            citations = {
                citation.content_version_id: citation
                for viewpoint in snapshot.viewpoints
                for citation in viewpoint.citations
                if citation.available
            }
            if not citations:
                raise AppError("analysis_has_no_available_citation", 409)
            input_sha256 = _digest(
                {
                    "analysis_run_id": str(snapshot.id),
                    "manifest_sha256": snapshot.manifest_sha256,
                    "body": body,
                    "citations": sorted(str(identity) for identity in citations),
                }
            )
            now = utcnow()
            entry = KnowledgeEntry(
                id=uuid4(),
                event_id=snapshot.event_id,
                source_analysis_run_id=snapshot.id,
                entry_type="analysis_snapshot",
                current_version=1,
                created_at=now,
                updated_at=now,
            )
            version = KnowledgeVersion(
                id=uuid4(),
                entry_id=entry.id,
                version=1,
                input_sha256=input_sha256,
                analysis_manifest_sha256=snapshot.manifest_sha256,
                title=title,
                body=body,
                search_text=normalize_search_text(f"{title}\n{body}"),
                created_at=now,
            )
            session.add_all([entry, version])
            session.flush()
            chunk_text = f"{title}\n{body}"
            session.add(
                KnowledgeChunk(
                    id=uuid4(),
                    version_id=version.id,
                    position=1,
                    text=chunk_text,
                    text_sha256=sha256(chunk_text.encode()).hexdigest(),
                    embedding=None,
                    embedding_model=None,
                    embedding_model_digest=None,
                    embedding_dimensions=None,
                    index_state="pending",
                    error_code=None,
                    created_at=now,
                    indexed_at=None,
                )
            )
            for position, identity in enumerate(sorted(citations), start=1):
                session.add(
                    KnowledgeCitation(
                        id=uuid4(),
                        version_id=version.id,
                        content_version_id=identity,
                        text_sha256=citations[identity].text_sha256,
                        position=position,
                    )
                )
            session.flush()
            audit(session, "knowledge_analysis_published", f"{analysis_run_id}:{entry.id}")
            return self._view(session, entry)

    def get(self, identity: UUID) -> KnowledgeEntryView:
        with self.factory() as session:
            return self._view(session, self._entry(session, identity))

    def withdraw_content(
        self,
        identity: UUID,
        data: ContentWithdrawalInput,
    ) -> ContentWithdrawalView:
        with self.factory.begin() as session:
            return self._withdraw_in_session(session, identity, data.reason)

    @staticmethod
    def _withdraw_in_session(
        session: Session,
        identity: UUID,
        reason: Literal["deleted", "purpose_revoked"],
    ) -> ContentWithdrawalView:
        withdrawn = withdraw_content(session, identity, reason)
        affected_entry_ids: set[UUID] = set()
        if withdrawn.version_ids:
            rows = session.execute(
                select(KnowledgeChunk, KnowledgeVersion.entry_id)
                .join(KnowledgeVersion, KnowledgeVersion.id == KnowledgeChunk.version_id)
                .join(
                    KnowledgeCitation,
                    KnowledgeCitation.version_id == KnowledgeVersion.id,
                )
                .where(KnowledgeCitation.content_version_id.in_(withdrawn.version_ids))
                .with_for_update()
            )
            for chunk, entry_id in rows:
                chunk.embedding = None
                chunk.embedding_model = None
                chunk.embedding_model_digest = None
                chunk.embedding_dimensions = None
                chunk.index_state = "deleted" if withdrawn.visibility == "deleted" else "stale"
                chunk.error_code = (
                    "content_deleted"
                    if withdrawn.visibility == "deleted"
                    else "content_purpose_revoked"
                )
                chunk.indexed_at = None
                affected_entry_ids.add(entry_id)
        if withdrawn.visibility == "deleted":
            schedule_raw_page_deletions(session, withdrawn.raw_page_ids)
        audit(
            session,
            "content_withdrawn",
            f"{identity}:{reason}:{len(affected_entry_ids)}",
        )
        return ContentWithdrawalView(
            id=withdrawn.id,
            visibility=withdrawn.visibility,
            affected_knowledge_entries=len(affected_entry_ids),
        )

    def export_withdrawal_manifest(self) -> ContentWithdrawalManifest:
        with self.factory() as session:
            return export_withdrawal_manifest(session)

    def replay_withdrawal_manifest(
        self,
        manifest: ContentWithdrawalManifest,
    ) -> ContentWithdrawalReplayView:
        applied = 0
        missing = 0
        with self.factory.begin() as session:
            for entry in manifest.entries:
                identity = apply_withdrawal_manifest_entry(session, entry)
                if identity is None:
                    missing += 1
                    continue
                reason: Literal["deleted", "purpose_revoked"] = (
                    "deleted" if entry.visibility == "deleted" else "purpose_revoked"
                )
                self._withdraw_in_session(session, identity, reason)
                applied += 1
        return ContentWithdrawalReplayView(applied=applied, missing=missing)

    def index(self, identity: UUID) -> KnowledgeEntryView:
        with self.factory() as session:
            entry = self._entry(session, identity)
            if self._view(session, entry).stale:
                raise AppError("knowledge_entry_stale", 409)
            version = self._version(session, entry)
            chunk = session.scalar(
                select(KnowledgeChunk).where(KnowledgeChunk.version_id == version.id)
            )
            if chunk is None:
                raise AppError("knowledge_chunk_missing", 500)
            if chunk.index_state == "ready":
                return self._view(session, entry)
            version_id = version.id
            chunk_id = chunk.id
            text = chunk.text
            text_sha256 = chunk.text_sha256
        if self.embedding_provider is None:
            raise AppError("embedding_not_configured", 503)
        try:
            batch = self.embedding_provider.embed([text])
        except EmbeddingProviderError as error:
            with self.factory.begin() as session:
                failed = session.get(KnowledgeChunk, chunk_id)
                if failed is not None and failed.text_sha256 == text_sha256:
                    failed.embedding = None
                    failed.embedding_model = None
                    failed.embedding_model_digest = None
                    failed.embedding_dimensions = None
                    failed.index_state = "failed"
                    failed.error_code = error.code
                    failed.indexed_at = None
            raise AppError(error.code, 503) from error

        if len(batch.vectors) != 1 or batch.dimensions != 1024:
            raise AppError("embedding_invalid_response", 503)
        with self.factory.begin() as session:
            entry = self._entry(session, identity)
            current_version = self._version(session, entry)
            chunk = session.scalar(
                select(KnowledgeChunk).where(KnowledgeChunk.id == chunk_id).with_for_update()
            )
            if (
                chunk is None
                or current_version.id != version_id
                or chunk.version_id != current_version.id
                or chunk.text_sha256 != text_sha256
            ):
                if chunk is not None:
                    chunk.embedding = None
                    chunk.index_state = "stale"
                    chunk.error_code = "knowledge_version_changed"
                    chunk.indexed_at = None
                raise AppError("knowledge_version_changed", 409)
            if self._view(session, entry).stale:
                chunk.embedding = None
                chunk.embedding_model = None
                chunk.embedding_model_digest = None
                chunk.embedding_dimensions = None
                chunk.index_state = "stale"
                chunk.error_code = "knowledge_entry_stale"
                chunk.indexed_at = None
                raise AppError("knowledge_entry_stale", 409)
            chunk.embedding = batch.vectors[0]
            chunk.embedding_model = batch.model
            chunk.embedding_model_digest = batch.model_digest
            chunk.embedding_dimensions = batch.dimensions
            chunk.index_state = "ready"
            chunk.error_code = None
            chunk.indexed_at = utcnow()
            session.flush()
            audit(session, "knowledge_semantic_indexed", f"{identity}:{version_id}")
            return self._view(session, entry)

    def search(
        self,
        query: str | None,
        event_id: UUID | None,
        limit: int,
        mode: Literal["exact_substring", "semantic"] = "exact_substring",
    ) -> KnowledgePage:
        normalized = normalize_search_text(query or "")
        if query is not None and not normalized:
            raise AppError("knowledge_query_empty", 422)
        if mode == "semantic":
            if not normalized:
                raise AppError("knowledge_query_required", 422)
            return self._semantic_search(normalized, event_id, limit)
        with self.factory() as session:
            statement = (
                select(KnowledgeEntry)
                .join(
                    KnowledgeVersion,
                    (KnowledgeVersion.entry_id == KnowledgeEntry.id)
                    & (KnowledgeVersion.version == KnowledgeEntry.current_version),
                )
                .order_by(KnowledgeEntry.created_at.desc(), KnowledgeEntry.id.desc())
            )
            if event_id is not None:
                statement = statement.where(KnowledgeEntry.event_id == event_id)
            if normalized:
                statement = statement.where(
                    KnowledgeVersion.search_text.contains(normalized, autoescape=True)
                )
            entries = list(session.scalars(statement))
            items: list[KnowledgeEntryView] = []
            for entry in entries:
                view = self._view(session, entry)
                if view.stale:
                    continue
                items.append(view)
                if len(items) == limit:
                    break
            return KnowledgePage(
                query_mode="exact_substring",
                query=normalized,
                items=items,
            )

    def _semantic_search(self, query: str, event_id: UUID | None, limit: int) -> KnowledgePage:
        if self.embedding_provider is None:
            raise AppError("embedding_not_configured", 503)
        try:
            batch = self.embedding_provider.embed([query])
        except EmbeddingProviderError as error:
            raise AppError(error.code, 503) from error
        if len(batch.vectors) != 1 or batch.dimensions != 1024:
            raise AppError("embedding_invalid_response", 503)

        with self.factory() as session:
            distance = KnowledgeChunk.embedding.cosine_distance(batch.vectors[0]).label("distance")
            statement = (
                select(KnowledgeEntry, distance)
                .join(
                    KnowledgeVersion,
                    (KnowledgeVersion.entry_id == KnowledgeEntry.id)
                    & (KnowledgeVersion.version == KnowledgeEntry.current_version),
                )
                .join(KnowledgeChunk, KnowledgeChunk.version_id == KnowledgeVersion.id)
                .where(
                    KnowledgeChunk.index_state == "ready",
                    KnowledgeChunk.embedding_model == batch.model,
                    KnowledgeChunk.embedding_model_digest == batch.model_digest,
                    KnowledgeChunk.embedding_dimensions == batch.dimensions,
                )
                .order_by(distance, KnowledgeEntry.id)
            )
            if event_id is not None:
                statement = statement.where(KnowledgeEntry.event_id == event_id)
            rows = session.execute(statement).all()
            items = []
            for entry, row_distance in rows:
                view = self._view(
                    session,
                    entry,
                    similarity=max(-1.0, min(1.0, 1.0 - float(row_distance))),
                )
                if view.stale:
                    continue
                items.append(view)
                if len(items) == limit:
                    break
            return KnowledgePage(
                query_mode="semantic",
                query=query,
                items=items,
            )

    def query(self, data: KnowledgeQuestion) -> KnowledgeAnswer:
        if isinstance(data, CommentCountQuestion):
            with self.factory() as session:
                statistics = controlled_comment_statistics(
                    session,
                    data.event_id,
                    data.since,
                    data.until,
                )
            return KnowledgeAnswer(
                kind="comment_count",
                status="answered",
                question=data.question,
                answer=(
                    f"{statistics.since.isoformat()} 至 {statistics.until.isoformat()}，"
                    f"事件范围内共有 {statistics.total} 条可见评论或回复。"
                ),
                method="controlled_statistics_v1",
                statistics=statistics,
            )

        assert isinstance(data, EvidenceQuestion)
        page = self.search(data.question, data.event_id, data.limit, data.mode)
        if not page.items:
            return KnowledgeAnswer(
                kind="evidence",
                status="unknown",
                question=data.question,
                answer="当前有效证据中没有找到可支持该问题的内容。",
                method="deterministic_retrieval_v1",
                unknown_reason="no_supported_evidence",
            )
        citations: dict[UUID, KnowledgeCitationView] = {}
        for entry in page.items:
            for citation in entry.citations:
                if citation.available:
                    citations.setdefault(citation.content_version_id, citation)
        if not citations:
            return KnowledgeAnswer(
                kind="evidence",
                status="unknown",
                question=data.question,
                answer="当前有效证据中没有找到可支持该问题的内容。",
                method="deterministic_retrieval_v1",
                unknown_reason="no_supported_evidence",
            )
        return KnowledgeAnswer(
            kind="evidence",
            status="answered",
            question=data.question,
            answer=page.items[0].body,
            method="deterministic_retrieval_v1",
            knowledge_entry_ids=[entry.id for entry in page.items],
            citations=list(citations.values()),
        )
