const TREND_LABELS: Record<string, string> = {
  emerging: "新出现",
  rising: "升温中",
  stable: "平稳",
  falling: "降温中",
  dormant: "低活跃",
};

const CONFIRMATION_LABELS: Record<string, string> = {
  corroborated: "多源印证",
  disputed: "存在争议",
  single_source: "单一来源",
  unverified: "待核实",
  insufficient: "证据不足",
};

const REASON_LABELS: Record<string, string> = {
  momentum: "传播动量正在增强",
  attention: "公众关注度正在上升",
  breadth: "来源覆盖正在扩大",
  latest: "出现新的事件进展",
  relevance: "与监控主题高度相关",
  momentum_rising: "传播动量正在增强",
  source_breadth_growing: "来源覆盖正在扩大",
  evidence_added: "新增可验证证据",
  heat_rising: "事件热度快速上升",
  trend_changed: "事件趋势发生变化",
  confirmation_changed: "事实确认状态发生变化",
};

const UPDATE_KIND_LABELS: Record<string, string> = {
  created: "事件建立",
  evidence_added: "新增证据",
  evidence_removed: "证据调整",
  heat_changed: "热度变化",
  trend_changed: "趋势变化",
  lifecycle_changed: "状态变化",
  corrected: "信息更正",
};

export function getRadarEventTitle(
  event: Pick<
    HotKeyAPI.RadarEventResponse,
    "title" | "title_zh" | "title_en" | "event_id"
  >,
) {
  return (
    event.title_zh?.trim() ||
    event.title?.trim() ||
    event.title_en?.trim() ||
    `事件 #${event.event_id ?? "—"}`
  );
}

export function trendLabel(value?: string) {
  return (value && TREND_LABELS[value]) || "持续观察";
}

export function confirmationLabel(value?: string) {
  return (value && CONFIRMATION_LABELS[value]) || "待核实";
}

export function reasonLabel(value?: string) {
  return (value && REASON_LABELS[value]) || "出现新的重要变化";
}

export function updateKindLabel(value?: string) {
  return (value && UPDATE_KIND_LABELS[value]) || "事件更新";
}

export function formatRadarTime(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function formatRadarScore(value?: number) {
  return value == null ? "—" : Math.round(value).toString();
}

export function trendTone(value?: string) {
  if (value === "emerging" || value === "rising") return "danger" as const;
  if (value === "falling" || value === "dormant") return "muted" as const;
  return "success" as const;
}

export function confirmationTone(value?: string) {
  if (value === "corroborated") return "success" as const;
  if (value === "disputed") return "danger" as const;
  return "warning" as const;
}
