"""Freeze and check the topic lifecycle for search and comment collection jobs."""

from uuid import UUID

from sqlalchemy.orm import Session

from jobs.models import Job

TOPIC_COLLECTION_SEQUENCE = "topic_collection_sequence"


def is_topic_collection(*, kind: str, configuration_ref: str) -> bool:
    return kind in {"keyword.search", "source.comments"} and configuration_ref.startswith("topic:")


def collection_sequence_in_transaction(
    session: Session, *, owner_id: UUID, configuration_ref: str
) -> int | None:
    # Runtime import avoids the monitors -> jobs service dependency at module load.
    from monitors.services import active_collection_sequence_in_transaction

    raw_id = configuration_ref.removeprefix("topic:")
    try:
        topic_id = UUID(raw_id)
    except ValueError:
        return None
    if str(topic_id) != raw_id:
        return None
    return active_collection_sequence_in_transaction(session, owner_id=owner_id, topic_id=topic_id)


def collection_topic_matches_in_transaction(session: Session, *, job: Job) -> bool:
    if not is_topic_collection(kind=job.kind, configuration_ref=job.configuration_ref):
        return True
    sequence = job.scope.get(TOPIC_COLLECTION_SEQUENCE)
    # Legacy tasks have no frozen lifecycle. Never bind them to a resumed topic.
    return (
        type(sequence) is int
        and sequence >= 0
        and sequence
        == collection_sequence_in_transaction(
            session, owner_id=job.owner_id, configuration_ref=job.configuration_ref
        )
    )
