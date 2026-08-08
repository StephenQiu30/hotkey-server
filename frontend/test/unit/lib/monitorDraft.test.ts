import { describe, expect, it } from "vitest";
import { MonitorRegion } from "@/lib/domainEnums";
import {
  MAX_MONITOR_SOURCES,
  buildMonitorDraftRequest,
  monitorToDraftForm,
  selectAllMonitorSources,
  toggleMonitorSource,
  validateMonitorDraft,
} from "@/lib/monitorDraft";

const baseForm = {
  name: "AI Agent 创建工具",
  description: "测试",
  rules: [{ key: "rule-1", ruleType: "keyword" as const, value: "Anthropic" }],
  languages: ["zh"],
  region: MonitorRegion.China,
  interval: 900,
  relevance: 60,
  event: 70,
  alertHeat: 70,
  alertMomentum: 55,
  alertBreadth: 25,
  alertWarning: 75,
  alertCritical: 90,
  alertCooldown: 60,
  alertEmailEnabled: false,
  alertEmailMinSeverity: "critical" as const,
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
    expect(request.config).toMatchObject({
      alert_min_heat: 70,
      alert_min_momentum: 55,
      alert_min_breadth: 25,
      alert_warning_threshold: 75,
      alert_critical_threshold: 90,
      alert_cooldown_minutes: 60,
      alert_email_enabled: false,
      alert_email_min_severity: "critical",
    });
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

  it("builds multilingual include, alias, and exclude rules without AI", () => {
    const request = buildMonitorDraftRequest({
      ...baseForm,
      languages: ["zh", "en"],
      rules: [
        { key: "1", ruleType: "keyword", value: "OpenAI" },
        { key: "2", ruleType: "entity", value: "开放人工智能" },
        { key: "3", ruleType: "exclude_keyword", value: "招聘" },
      ],
    });
    expect(request.config.languages).toEqual(["zh", "en"]);
    expect(request.rules).toEqual([
      expect.objectContaining({
        rule_type: "keyword",
        value: "OpenAI",
        weight: 100,
      }),
      expect.objectContaining({
        rule_type: "entity",
        value: "开放人工智能",
        weight: 100,
      }),
      expect.objectContaining({
        rule_type: "exclude_keyword",
        value: "招聘",
        weight: 0,
      }),
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
    expect(
      validateMonitorDraft({ ...baseForm, alertWarning: 95, alertCritical: 90 })
    ).toBe("严重级别阈值需满足 0 ≤ 警告 ≤ 严重 ≤ 100");
  });

  it("prefills an editable form from the newest draft without losing source identity", () => {
    const form = monitorToDraftForm({
      name: "AI releases",
      description: "Official launches",
      published: {
        revision: 1,
        rules: [{ value: "old" }],
        sources: [{ source_connection_id: 4 }],
      },
      draft: {
        revision: 2,
        collection_interval_seconds: 600,
        event_threshold: 80,
        alert_min_heat: 72,
        alert_min_momentum: 60,
        alert_min_breadth: 50,
        alert_warning_threshold: 78,
        alert_critical_threshold: 92,
        alert_cooldown_minutes: 90,
        alert_email_enabled: true,
        alert_email_min_severity: "warning",
        languages: ["en"],
        regions: ["US"],
        relevance_threshold: 72,
        retention_days: 45,
        rules: [
          {
            rule_type: "keyword",
            value: "OpenAI",
            origin: "user",
            enabled: true,
          },
        ],
        sources: [{ source_connection_id: 7, enabled: true }],
      },
    });
    expect(form).toEqual({
      name: "AI releases",
      description: "Official launches",
      rules: [
        expect.objectContaining({ ruleType: "keyword", value: "OpenAI" }),
      ],
      languages: ["en"],
      region: MonitorRegion.UnitedStates,
      interval: 600,
      relevance: 72,
      event: 80,
      alertHeat: 72,
      alertMomentum: 60,
      alertBreadth: 50,
      alertWarning: 78,
      alertCritical: 92,
      alertCooldown: 90,
      alertEmailEnabled: true,
      alertEmailMinSeverity: "warning",
      retention: 45,
      sourceIds: [7],
    });
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
