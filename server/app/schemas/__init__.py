"""Pydantic schemas will be introduced by the execution plans."""
from server.app.schemas.ai_analysis import AiAnalysisRead
from server.app.schemas.check_run import CheckRunCreate, CheckRunRead
from server.app.schemas.hotspot import HotspotRead
from server.app.schemas.keyword import KeywordCreate, KeywordRead, KeywordUpdate
from server.app.schemas.notification import NotificationRead
from server.app.schemas.report import ReportCreate, ReportRead
from server.app.schemas.search import SearchCreate, SearchRead, SearchResultRead
from server.app.schemas.setting import SettingRead, SettingUpsert
from server.app.schemas.source import SourceCreate, SourceRead, SourceUpdate

__all__ = [
    "AiAnalysisRead",
    "CheckRunCreate",
    "CheckRunRead",
    "HotspotRead",
    "KeywordCreate",
    "KeywordRead",
    "KeywordUpdate",
    "NotificationRead",
    "ReportCreate",
    "ReportRead",
    "SearchCreate",
    "SearchRead",
    "SearchResultRead",
    "SettingRead",
    "SettingUpsert",
    "SourceCreate",
    "SourceRead",
    "SourceUpdate",
]
