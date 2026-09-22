import { describe, expect, it } from "vitest";

import {
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
});
