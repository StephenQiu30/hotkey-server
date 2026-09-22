import { describe, expect, it } from "vitest";

import {
  contentScopeLabel,
  contentScopeNotice,
  formatMetric,
  formatTime,
  hasUnknownMetrics,
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

  it("labels partial and media-only content without promoting it to full text", () => {
    expect(contentScopeLabel("full")).toBe("完整原文");
    expect(contentScopeLabel("summary")).toBe("摘要");
    expect(contentScopeLabel("truncated")).toBe("已截断");
    expect(contentScopeLabel("media_only")).toBe("仅媒体");
    expect(contentScopeNotice("summary")).toContain("不代表完整原文");
    expect(contentScopeNotice("media_only")).toContain("未保存或理解媒体内容");
  });
});
