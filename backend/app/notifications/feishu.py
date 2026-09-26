from __future__ import annotations

import base64
import hashlib
import hmac
import logging
from datetime import datetime
from pathlib import Path
from typing import Any
from urllib.parse import quote, urlsplit
from uuid import UUID
from zoneinfo import ZoneInfo

import httpx
from pydantic import SecretStr

from reports.schemas import DailyReportData


class FeishuDeliveryError(Exception):
    def __init__(self, code: str, *, uncertain: bool = False) -> None:
        self.code = code
        self.uncertain = uncertain
        super().__init__(code)


def validate_webhook_url(value: str) -> str:
    try:
        parsed = urlsplit(value)
        port = parsed.port
    except ValueError as error:
        raise ValueError("invalid Feishu webhook URL") from error
    if (
        parsed.scheme != "https"
        or parsed.hostname not in {"open.feishu.cn", "open.larksuite.com"}
        or port is not None
        or parsed.username is not None
        or parsed.password is not None
        or not parsed.path.startswith("/open-apis/bot/v2/hook/")
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError("invalid Feishu webhook URL")
    return value


def sign(timestamp: int, secret: SecretStr) -> str:
    key = f"{timestamp}\n{secret.get_secret_value()}".encode()
    return base64.b64encode(hmac.new(key, digestmod=hashlib.sha256).digest()).decode()


def report_card(
    data: DailyReportData,
    *,
    report_id: UUID,
    generator: str,
    web_base_url: str,
    vault_name: str,
    export_relative_path: str | None,
) -> dict[str, Any]:
    date = data.window_end.astimezone(ZoneInfo("Asia/Shanghai")).date().isoformat()
    overview = data.overview
    points = [
        f"相关帖子 {overview.posts.current} 条、评论 {overview.comments.current} 条",
        f"新增帖子变化 {overview.posts.delta:+d}、评论变化 {overview.comments.delta:+d}",
    ]
    points.extend(item.text for item in data.narratives.get("overview", ())[:2])
    generator_label = "模型版" if generator == "model" else "模板版"
    elements: list[dict[str, Any]] = [
        {
            "tag": "div",
            "text": {
                "tag": "lark_md",
                "content": f"**日期**: {date}　**生成方式**: {generator_label}",
            },
        },
        {
            "tag": "div",
            "text": {
                "tag": "lark_md",
                "content": "**概览要点**\n" + "\n".join(f"• {point}" for point in points),
            },
        },
    ]
    linked_contents = [item for item in data.top_contents if item.url is not None][:3]
    for index, item in enumerate(linked_contents, start=1):
        title = item.title.replace("[", "\\[").replace("]", "\\]")
        content = f"{index}. [{title}]({item.url})"
        elements.append({"tag": "div", "text": {"tag": "lark_md", "content": content}})
    if not linked_contents:
        elements.append({"tag": "div", "text": {"tag": "lark_md", "content": "暂无可用原帖链接"}})
    actions = [
        {
            "tag": "button",
            "text": {"tag": "plain_text", "content": "查看报告"},
            "type": "primary",
            "url": f"{web_base_url.rstrip('/')}/reports/{report_id}",
        }
    ]
    if export_relative_path is not None:
        stem = Path(export_relative_path).with_suffix("").as_posix()
        url = f"obsidian://open?vault={quote(vault_name, safe='')}&file={quote(stem, safe='')}"
        actions.append(
            {
                "tag": "button",
                "text": {"tag": "plain_text", "content": "在 Obsidian 打开"},
                "url": url,
            }
        )
    elements.append({"tag": "action", "actions": actions})
    return {
        "msg_type": "interactive",
        "card": {
            "config": {"wide_screen_mode": True},
            "header": {"title": {"tag": "plain_text", "content": f"{data.topic_name} 日报"}},
            "elements": elements,
        },
    }


class FeishuWebhook:
    def __init__(self, *, url: SecretStr, secret: SecretStr | None, client: httpx.Client) -> None:
        # HTTPX's INFO completion log contains the webhook token in the URL.
        logging.getLogger("httpx").setLevel(logging.WARNING)
        logging.getLogger("httpcore").setLevel(logging.WARNING)
        self._url = validate_webhook_url(url.get_secret_value())
        self._secret = secret
        self._client = client

    def send(self, card: dict[str, Any], *, now: datetime) -> None:
        body = dict(card)
        if self._secret is not None and self._secret.get_secret_value():
            timestamp = int(now.timestamp())
            body.update(timestamp=str(timestamp), sign=sign(timestamp, self._secret))
        try:
            response = self._client.post(self._url, json=body, follow_redirects=False)
        except (httpx.ConnectError, httpx.ConnectTimeout):
            raise FeishuDeliveryError("feishu_connect_failed") from None
        except httpx.RequestError:
            raise FeishuDeliveryError("feishu_request_uncertain", uncertain=True) from None
        if 400 <= response.status_code < 500:
            raise FeishuDeliveryError("feishu_client_error")
        if response.status_code >= 500 or 300 <= response.status_code < 400:
            raise FeishuDeliveryError("feishu_response_uncertain", uncertain=True)
        try:
            result = response.json()
        except ValueError as error:
            raise FeishuDeliveryError("feishu_response_uncertain", uncertain=True) from error
        if not isinstance(result, dict) or result.get("code") != 0:
            raise FeishuDeliveryError("feishu_api_error")
