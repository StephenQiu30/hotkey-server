from __future__ import annotations

from pydantic import BaseModel


class Page(BaseModel):
    items: list
    limit: int
    offset: int
