import json
import re
import unicodedata
from hashlib import sha256
from uuid import UUID, uuid4

from sqlalchemy import select
from sqlalchemy.orm import Session, sessionmaker

from analysis.schemas import AnalysisRunView
from analysis.services import analysis_knowledge_snapshot
from audit.services import audit
from contents.services import content_version_contexts, current_content_version_ids
from core.clock import utcnow
from core.errors import AppError
from events.services import event_title_for_knowledge
from knowledge.models import KnowledgeCitation, KnowledgeEntry, KnowledgeVersion
from knowledge.schemas import KnowledgeCitationView, KnowledgeEntryView, KnowledgePage


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
    def __init__(self, factory: sessionmaker[Session]):
        self.factory = factory

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
    def _view(cls, session: Session, entry: KnowledgeEntry) -> KnowledgeEntryView:
        version = cls._version(session, entry)
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

    def search(self, query: str | None, event_id: UUID | None, limit: int) -> KnowledgePage:
        normalized = normalize_search_text(query or "")
        if query is not None and not normalized:
            raise AppError("knowledge_query_empty", 422)
        with self.factory() as session:
            statement = (
                select(KnowledgeEntry)
                .join(
                    KnowledgeVersion,
                    (KnowledgeVersion.entry_id == KnowledgeEntry.id)
                    & (KnowledgeVersion.version == KnowledgeEntry.current_version),
                )
                .order_by(KnowledgeEntry.created_at.desc(), KnowledgeEntry.id.desc())
                .limit(limit)
            )
            if event_id is not None:
                statement = statement.where(KnowledgeEntry.event_id == event_id)
            if normalized:
                statement = statement.where(
                    KnowledgeVersion.search_text.contains(normalized, autoescape=True)
                )
            entries = list(session.scalars(statement))
            return KnowledgePage(
                query_mode="exact_substring",
                query=normalized,
                items=[self._view(session, entry) for entry in entries],
            )
