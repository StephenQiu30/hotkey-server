from uuid import uuid4

from sqlalchemy.orm import Session

from audit.models import Audit
from core.clock import utcnow


def audit(session: Session, action: str, target: str) -> None:
    session.add(Audit(id=uuid4(), action=action, target=target, occurred_at=utcnow()))
