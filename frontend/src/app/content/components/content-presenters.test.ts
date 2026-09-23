import { describe, expect, it } from "vitest";

import {
  contentScopeLabel,
  contentScopeNotice,
  formatMetric,
  formatTime,
  hasUnknownMetrics,
  scanKindLabel,
  visibilityStatusLabel,
  visibilityStatusNotice,
} from "./content-presenters";

const completeMetrics: HotKeyAPI.ContentMetricView = {
  like_count: 0,
  comment_count: 1,
  repost_count: 2,
  view_count: 3,
  play_count: 4,
  danmaku_count: 5,
};

describe("content presenters", () => {
  it("keeps an observed zero distinct from an unavailable metric", () => {
    expect(formatMetric(0)).toBe("0");
    expect(formatMetric(null)).toBe("未知");
  });

  it("marks only records with unavailable metrics as incomplete", () => {
    expect(hasUnknownMetrics(completeMetrics)).toBe(false);
    expect(hasUnknownMetrics({ ...completeMetrics, view_count: null })).toBe(
      true,
    );
  });

  it("does not invent an unknown timestamp", () => {
    expect(formatTime(null)).toBe("未知");
  });

  it("marks historical discovery without presenting it as a new event", () => {
    expect(scanKindLabel("backfill")).toBe("历史回补 · 非新发生");
    expect(scanKindLabel("new_scan")).toBe("追新");
    expect(scanKindLabel(null)).toBe("采集类型未标注");
  });

  it("labels partial and media-only content without promoting it to full text", () => {
    expect(contentScopeLabel("full")).toBe("完整原文");
    expect(contentScopeLabel("summary")).toBe("摘要");
    expect(contentScopeLabel("truncated")).toBe("已截断");
    expect(contentScopeLabel("media_only")).toBe("仅媒体");
    expect(contentScopeNotice("summary")).toContain("不代表完整原文");
    expect(contentScopeNotice("media_only")).toContain("未保存或理解媒体内容");
  });

  it("keeps source deletion distinct from temporary and unknown failures", () => {
    expect(visibilityStatusLabel("deleted")).toBe("来源已删除");
    expect(visibilityStatusLabel("transient_failure")).toBe("来源暂时不可达");
    expect(visibilityStatusLabel("unknown")).toBe("来源状态未知");
    expect(visibilityStatusNotice("transient_failure")).toContain(
      "保留最后成功资料",
    );
    expect(visibilityStatusNotice("unknown")).toContain("不能据此认定删除");
  });
});
