import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { KeywordGroupField } from "./keyword-group-field";
import {
  selectableTopicSources,
  TopicSettingsFields,
} from "./topic-settings-fields";

const entry: HotKeyAPI.SourceEntryPointView = {
  status: "pending_verification",
  last_checked_at: null,
  last_persisted_success_at: null,
  stop_reason: null,
  next_action: "完成持久读取验证。",
};

function source(
  status: HotKeyAPI.SourceCapabilityStatus,
): HotKeyAPI.SourcePlatformView {
  return {
    source_key: "hackernews",
    display_name: "Hacker News",
    rollout_role: "required",
    status: "pending_verification",
    connection_version: 1,
    has_credentials: false,
    connection_id: "00000000-0000-4000-8000-000000000001",
    connection_status: "active",
    credential_configured: false,
    credential_update_available: false,
    allowed_hosts: [],
    capabilities: [
      {
        capability: "search",
        display_name: "搜索",
        manual: entry,
        scheduled: { ...entry, status },
      },
    ],
  };
}

describe("topic source settings", () => {
  it("distinguishes a selectable pending source from restricted and missing sources", () => {
    const pending = selectableTopicSources([source("pending_verification")]);
    expect(pending[0].selectable).toBe(true);
    expect(pending[0].reason).toContain("待真实采集验证");
    const restricted = selectableTopicSources(
      [source("restricted")],
      ["missing"],
    );
    expect(restricted[0].selectable).toBe(false);
    expect(restricted[0].reason).toContain("持久读取验证");
    expect(restricted[1].selectable).toBe(false);
    const html = renderToStaticMarkup(
      createElement(TopicSettingsFields, {
        sourceOptions: restricted,
        sourceKeys: [],
        onSourceKeysChange: vi.fn(),
        collectionIntervalSeconds: 1800,
        onCollectionIntervalSecondsChange: vi.fn(),
        reportTime: "09:00",
        onReportTimeChange: vi.fn(),
        weeklyReportEnabled: false,
        onWeeklyReportEnabledChange: vi.fn(),
        notificationTargets: "",
        onNotificationTargetsChange: vi.fn(),
        disabled: false,
      }),
    );
    expect(html).toContain("完成持久读取验证");
    expect(html).toContain("disabled");
  });

  it("renders 422 messages beside the affected controls", () => {
    const keyword = renderToStaticMarkup(
      createElement(KeywordGroupField, {
        id: "match-any",
        label: "任意命中",
        description: "填写关键词",
        value: "AI",
        onChange: vi.fn(),
        error: "关键词过长",
      }),
    );
    expect(keyword).toContain("关键词过长");
    expect(keyword).toContain('aria-invalid="true"');
    const settings = renderToStaticMarkup(
      createElement(TopicSettingsFields, {
        sourceOptions: [],
        sourceKeys: [],
        onSourceKeysChange: vi.fn(),
        collectionIntervalSeconds: 599,
        onCollectionIntervalSecondsChange: vi.fn(),
        reportTime: "",
        onReportTimeChange: vi.fn(),
        weeklyReportEnabled: false,
        onWeeklyReportEnabledChange: vi.fn(),
        notificationTargets: "",
        onNotificationTargetsChange: vi.fn(),
        disabled: false,
        fieldErrors: { collection_interval_seconds: "必须不少于 600" },
      }),
    );
    expect(settings).toContain("必须不少于 600");
    expect(settings).toContain('aria-invalid="true"');
  });
});
