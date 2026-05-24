from __future__ import annotations

from fastapi import APIRouter, Depends
from sqlalchemy import select
from sqlalchemy.orm import Session

from server.app.db.session import get_session
from server.app.core.security import require_permission
from server.app.models.setting import Setting
from server.app.schemas.setting import SettingRead, SettingUpsert

router = APIRouter(prefix="/api/settings", tags=["settings"])


@router.get("", response_model=list[SettingRead])
def list_settings(session: Session = Depends(get_session)) -> list[Setting]:
    return list(session.scalars(select(Setting).order_by(Setting.key)))


@router.put("/{key}", response_model=SettingRead, dependencies=[Depends(require_permission("settings.manage"))])
def upsert_setting(key: str, payload: SettingUpsert, session: Session = Depends(get_session)) -> Setting:
    setting = session.get(Setting, key)
    if setting is None:
        setting = Setting(key=key, value=payload.value, description=payload.description)
        session.add(setting)
    else:
        setting.value = payload.value
        setting.description = payload.description
    session.commit()
    session.refresh(setting)
    return setting
