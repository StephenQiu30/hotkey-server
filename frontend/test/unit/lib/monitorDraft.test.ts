import { describe, expect, it } from "vitest";
import { MonitorRegion } from "@/lib/domainEnums";
import {
  MAX_MONITOR_SOURCES,
  buildMonitorDraftRequest,
  selectAllMonitorSources,
  toggleMonitorSource,
  validateMonitorDraft,
} from "@/lib/monitorDraft";

const baseForm = {
  name: "AI Agent 创建工具",
  description: "测试",
  query: "Anthropic",
  language: "zh",
  region: MonitorRegion.China,
  interval: 900,
  relevance: 60,
  event: 70,
  retention: 30,
  sourceIds: [1],
};

describe("monitor draft contract", () => {
  it("maps the global option to an empty regions array", () => {
    const request = buildMonitorDraftRequest({
      ...baseForm,
      region: MonitorRegion.Global,
    });

    expect(request.config.regions).toEqual([]);
  });

  it("builds the source priorities from the selected source order", () => {
    const request = buildMonitorDraftRequest({
      ...baseForm,
      sourceIds: [7, 3],
    });

    expect(request.sources).toEqual([
      { source_connection_id: 7, enabled: true, priority: 1 },
      { source_connection_id: 3, enabled: true, priority: 2 },
    ]);
  });

  it("reports client-side constraints before calling the API", () => {
    expect(
      validateMonitorDraft({
        ...baseForm,
        sourceIds: Array.from({ length: 11 }, (_, index) => index + 1),
      })
    ).toBe("数据来源最多选择 10 个");
    expect(validateMonitorDraft({ ...baseForm, interval: 240 })).toBe(
      "采集间隔需为 300–86400 秒，并且是 60 的倍数"
    );
    expect(validateMonitorDraft({ ...baseForm, relevance: 59 })).toBe(
      "相关性阈值需为 60–100"
    );
  });
});

describe("limited monitor source selection", () => {
  const sourceIds = Array.from({ length: 14 }, (_, index) => index + 1);

  it("selects only the backend-supported maximum", () => {
    expect(selectAllMonitorSources(sourceIds)).toEqual(
      sourceIds.slice(0, MAX_MONITOR_SOURCES)
    );
  });

  it("does not add an eleventh source", () => {
    const selected = sourceIds.slice(0, MAX_MONITOR_SOURCES);
    expect(toggleMonitorSource(selected, 11, true)).toEqual(selected);
  });

  it("deduplicates additions and still allows deselection at the limit", () => {
    expect(toggleMonitorSource([1, 2], 2, true)).toEqual([1, 2]);
    expect(toggleMonitorSource(sourceIds.slice(0, 10), 5, false)).toEqual([
      1, 2, 3, 4, 6, 7, 8, 9, 10,
    ]);
  });
});
