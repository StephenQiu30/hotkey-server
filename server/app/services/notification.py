from __future__ import annotations

import smtplib
from datetime import datetime, timezone
from email.message import EmailMessage

from sqlalchemy.orm import Session

from server.app.core.settings import settings
from server.app.models.ai_analysis import AiAnalysis
from server.app.models.hotspot import Hotspot
from server.app.models.notification import Notification
from server.app.models.report import Report


def notify_hotspot(session: Session, hotspot: Hotspot, analysis: AiAnalysis) -> Notification:
    recipient = settings.smtp_to_email
    notification = Notification(hotspot_id=hotspot.id, channel="email", recipient=recipient)
    if not _smtp_configured():
        notification.status = "skipped"
        notification.error_message = "SMTP is not configured."
        session.add(notification)
        return notification

    try:
        _send_email(hotspot, analysis)
        notification.status = "sent"
        notification.sent_at = datetime.now(timezone.utc)
    except Exception as exc:  # noqa: BLE001
        notification.status = "failed"
        notification.error_message = str(exc)
    session.add(notification)
    return notification


def notify_report(session: Session, report: Report) -> Notification:
    recipient = settings.smtp_to_email
    notification = Notification(report_id=report.id, channel="email", recipient=recipient)
    if not _smtp_configured():
        notification.status = "skipped"
        notification.error_message = "SMTP is not configured."
        session.add(notification)
        return notification

    try:
        _send_report_email(report)
        notification.status = "sent"
        notification.sent_at = datetime.now(timezone.utc)
    except Exception as exc:  # noqa: BLE001
        notification.status = "failed"
        notification.error_message = str(exc)
    session.add(notification)
    return notification


def _smtp_configured() -> bool:
    return bool(settings.smtp_host and settings.smtp_from_email and settings.smtp_to_email)


def _send_email(hotspot: Hotspot, analysis: AiAnalysis) -> None:
    message = EmailMessage()
    message["Subject"] = f"[AI Hotspot] {hotspot.title}"
    message["From"] = settings.smtp_from_email or ""
    message["To"] = settings.smtp_to_email or ""
    message.set_content(
        "\n".join(
            [
                hotspot.title,
                "",
                analysis.summary or "",
                "",
                f"Importance: {analysis.importance}",
                f"Relevance: {analysis.relevance_score}",
                f"Reason: {analysis.relevance_reason or ''}",
                f"Source: {hotspot.url}",
            ]
        )
    )
    with smtplib.SMTP(settings.smtp_host or "", settings.smtp_port, timeout=20) as smtp:
        if settings.smtp_use_tls:
            smtp.starttls()
        if settings.smtp_username and settings.smtp_password:
            smtp.login(settings.smtp_username, settings.smtp_password)
        smtp.send_message(message)


def _send_report_email(report: Report) -> None:
    message = EmailMessage()
    message["Subject"] = report.subject
    message["From"] = settings.smtp_from_email or ""
    message["To"] = settings.smtp_to_email or ""
    message.set_content(report.content)
    with smtplib.SMTP(settings.smtp_host or "", settings.smtp_port, timeout=20) as smtp:
        if settings.smtp_use_tls:
            smtp.starttls()
        if settings.smtp_username and settings.smtp_password:
            smtp.login(settings.smtp_username, settings.smtp_password)
        smtp.send_message(message)
