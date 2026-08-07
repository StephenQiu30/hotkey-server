import { MonitorRegion } from "@/lib/domainEnums";

export const MAX_MONITOR_SOURCES = 10;
export const MAX_MONITOR_RULES = 100;

export const MONITOR_RULE_TYPES = [
  { value: "keyword", label: "关键词" },
  { value: "phrase", label: "短语" },
  { value: "entity", label: "别名 / 实体" },
  { value: "exclude_keyword", label: "排除词" },
] as const;

export type MonitorRuleType = (typeof MONITOR_RULE_TYPES)[number]["value"];

export type MonitorDraftRule = {
  key: string;
  ruleType: MonitorRuleType;
  value: string;
};

export const MONITOR_LIMITS = {
  interval: { min: 300, max: 86_400, step: 60 },
  relevance: { min: 60, max: 100 },
  event: { min: 0, max: 100 },
  retention: { min: 1, max: 3_650 },
} as const;

export type MonitorDraftForm = {
  name: string;
  description: string;
  rules: MonitorDraftRule[];
  languages: string[];
  region: MonitorRegion;
  interval: number;
  relevance: number;
  event: number;
  retention: number;
  sourceIds: number[];
};

let ruleSequence = 0;

export function createMonitorRule(
  ruleType: MonitorRuleType = "keyword",
  value = "",
): MonitorDraftRule {
  ruleSequence += 1;
  return { key: `rule-${ruleSequence}`, ruleType, value };
}

const runeLength = (value: string) => Array.from(value.trim()).length;

export function validateMonitorDraft(form: MonitorDraftForm): string | null {
  if (!form.name.trim()) return "请填写监控名称";
  if (runeLength(form.name) > 120) return "监控名称不能超过 120 个字符";
  if (runeLength(form.description) > 2_000) return "说明不能超过 2000 个字符";
  if (!form.languages.length) return "请至少选择 1 种语言";
  if (form.languages.length > 8) return "语言最多选择 8 种";
  if (!form.rules.length) return "请至少添加 1 条包含规则";
  if (form.rules.length > MAX_MONITOR_RULES) return `规则最多 ${MAX_MONITOR_RULES} 条`;
  if (!form.rules.some((rule) => rule.ruleType !== "exclude_keyword" && rule.value.trim()))
    return "请至少添加 1 条关键词、短语或别名";
  const seen = new Set<string>();
  for (const rule of form.rules) {
    if (!rule.value.trim()) return "规则内容不能为空";
    if (runeLength(rule.value) > 160) return "单条规则不能超过 160 个字符";
    const identity = `${rule.ruleType}:${rule.value.trim().toLowerCase()}`;
    if (seen.has(identity)) return "请删除重复规则";
    seen.add(identity);
  }
  if (!form.sourceIds.length) return "请至少选择 1 个数据来源";
  if (form.sourceIds.length > MAX_MONITOR_SOURCES)
    return `数据来源最多选择 ${MAX_MONITOR_SOURCES} 个`;
  if (
    form.interval < MONITOR_LIMITS.interval.min ||
    form.interval > MONITOR_LIMITS.interval.max ||
    form.interval % MONITOR_LIMITS.interval.step !== 0
  )
    return "采集间隔需为 300–86400 秒，并且是 60 的倍数";
  if (form.relevance < MONITOR_LIMITS.relevance.min || form.relevance > MONITOR_LIMITS.relevance.max)
    return "相关性阈值需为 60–100";
  if (form.event < MONITOR_LIMITS.event.min || form.event > MONITOR_LIMITS.event.max)
    return "事件阈值需为 0–100";
  if (form.retention < MONITOR_LIMITS.retention.min || form.retention > MONITOR_LIMITS.retention.max)
    return "保留天数需为 1–3650";
  return null;
}

export function buildMonitorDraftRequest(form: MonitorDraftForm) {
  return {
    name: form.name.trim(),
    description: form.description.trim() || undefined,
    config: {
      collection_interval_seconds: form.interval,
      event_threshold: form.event,
      languages: form.languages,
      regions: form.region === MonitorRegion.Global ? [] : [form.region],
      relevance_threshold: form.relevance,
      retention_days: form.retention,
      timezone: "Asia/Shanghai",
    },
    rules: form.rules.map((rule, index) => ({
      rule_type: rule.ruleType,
      operator: "contains" as const,
      value: rule.value.trim(),
      enabled: true,
      priority: index + 1,
      weight: rule.ruleType === "exclude_keyword" ? 0 : 100,
    })),
    sources: form.sourceIds.map((source_connection_id, index) => ({
      source_connection_id,
      enabled: true,
      priority: index + 1,
    })),
  };
}

export function monitorToDraftForm(monitor: HotKeyAPI.MonitorResponse): MonitorDraftForm {
  const config = monitor.draft ?? monitor.published;
  const region = config?.regions?.[0];
  const editableRules = (config?.rules ?? []).flatMap((rule) => {
    const ruleType = rule.rule_type;
    const supported = MONITOR_RULE_TYPES.some((option) => option.value === ruleType);
    return rule.enabled !== false && (!rule.origin || rule.origin === "user") && supported
      ? [{ ...rule, rule_type: ruleType as MonitorRuleType }]
      : [];
  });
  return {
    name: monitor.name ?? "",
    description: monitor.description ?? "",
    rules: editableRules.length
      ? editableRules.map((rule) => createMonitorRule(rule.rule_type, rule.value ?? ""))
      : [createMonitorRule()],
    languages: config?.languages?.length ? config.languages : ["zh"],
    region:
      region === MonitorRegion.China || region === MonitorRegion.UnitedStates
        ? region
        : MonitorRegion.Global,
    interval: config?.collection_interval_seconds ?? 900,
    relevance: config?.relevance_threshold ?? 60,
    event: config?.event_threshold ?? 70,
    retention: config?.retention_days ?? 30,
    sourceIds:
      config?.sources?.flatMap((source) =>
        source.enabled !== false && source.source_connection_id != null
          ? [source.source_connection_id]
          : [],
      ) ?? [],
  };
}

export function selectAllMonitorSources(availableIds: number[]): number[] {
  return Array.from(new Set(availableIds)).slice(0, MAX_MONITOR_SOURCES);
}

export function toggleMonitorSource(selectedIds: number[], id: number, checked: boolean): number[] {
  if (!checked) return selectedIds.filter((value) => value !== id);
  if (selectedIds.includes(id) || selectedIds.length >= MAX_MONITOR_SOURCES) return selectedIds;
  return [...selectedIds, id];
}

export function toggleMonitorLanguage(selected: string[], language: string, checked: boolean): string[] {
  if (!checked) return selected.filter((value) => value !== language);
  return selected.includes(language) ? selected : [...selected, language];
}
