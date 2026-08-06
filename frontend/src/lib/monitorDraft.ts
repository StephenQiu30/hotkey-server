import { MonitorRegion } from "@/lib/domainEnums";

export const MAX_MONITOR_SOURCES = 10;

export const MONITOR_LIMITS = {
  interval: { min: 300, max: 86_400, step: 60 },
  relevance: { min: 60, max: 100 },
  event: { min: 0, max: 100 },
  retention: { min: 1, max: 3_650 },
} as const;

export type MonitorDraftForm = {
  name: string;
  description: string;
  query: string;
  language: string;
  region: MonitorRegion;
  interval: number;
  relevance: number;
  event: number;
  retention: number;
  sourceIds: number[];
};

const runeLength = (value: string) => Array.from(value.trim()).length;

export function validateMonitorDraft(form: MonitorDraftForm): string | null {
  if (!form.name.trim()) return "请填写监控名称";
  if (runeLength(form.name) > 120) return "监控名称不能超过 120 个字符";
  if (!form.query.trim()) return "请填写关键词规则";
  if (runeLength(form.query) > 160) return "关键词规则不能超过 160 个字符";
  if (runeLength(form.description) > 2_000) return "说明不能超过 2000 个字符";
  if (!form.sourceIds.length) return "请至少选择 1 个数据来源";
  if (form.sourceIds.length > MAX_MONITOR_SOURCES)
    return `数据来源最多选择 ${MAX_MONITOR_SOURCES} 个`;
  if (
    form.interval < MONITOR_LIMITS.interval.min ||
    form.interval > MONITOR_LIMITS.interval.max ||
    form.interval % MONITOR_LIMITS.interval.step !== 0
  )
    return "采集间隔需为 300–86400 秒，并且是 60 的倍数";
  if (
    form.relevance < MONITOR_LIMITS.relevance.min ||
    form.relevance > MONITOR_LIMITS.relevance.max
  )
    return "相关性阈值需为 60–100";
  if (
    form.event < MONITOR_LIMITS.event.min ||
    form.event > MONITOR_LIMITS.event.max
  )
    return "事件阈值需为 0–100";
  if (
    form.retention < MONITOR_LIMITS.retention.min ||
    form.retention > MONITOR_LIMITS.retention.max
  )
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
      languages: [form.language],
      regions: form.region === MonitorRegion.Global ? [] : [form.region],
      relevance_threshold: form.relevance,
      retention_days: form.retention,
      timezone: "Asia/Shanghai",
    },
    rules: [
      {
        rule_type: "keyword" as const,
        operator: "contains" as const,
        value: form.query.trim(),
        enabled: true,
        priority: 1,
        weight: 1,
      },
    ],
    sources: form.sourceIds.map((source_connection_id, index) => ({
      source_connection_id,
      enabled: true,
      priority: index + 1,
    })),
  };
}

export function selectAllMonitorSources(availableIds: number[]): number[] {
  return Array.from(new Set(availableIds)).slice(0, MAX_MONITOR_SOURCES);
}

export function toggleMonitorSource(
  selectedIds: number[],
  id: number,
  checked: boolean
): number[] {
  if (!checked) return selectedIds.filter((value) => value !== id);
  if (selectedIds.includes(id) || selectedIds.length >= MAX_MONITOR_SOURCES)
    return selectedIds;
  return [...selectedIds, id];
}
