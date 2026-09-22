const DATE_TIME_FORMATTER = new Intl.DateTimeFormat("zh-CN", {
  dateStyle: "medium",
  timeStyle: "short",
});

export const METRIC_LABELS: ReadonlyArray<
  readonly [keyof HotKeyAPI.ContentMetricView, string]
> = [
  ["like_count", "点赞"],
  ["comment_count", "评论"],
  ["repost_count", "转发"],
  ["view_count", "浏览"],
  ["play_count", "播放"],
  ["danmaku_count", "弹幕"],
];

export function formatMetric(value: number | null): string {
  return value === null ? "未知" : new Intl.NumberFormat("zh-CN").format(value);
}

export function formatTime(value: string | null): string {
  return value === null ? "未知" : DATE_TIME_FORMATTER.format(new Date(value));
}

export function hasUnknownMetrics(
  metrics: HotKeyAPI.ContentMetricView,
): boolean {
  return METRIC_LABELS.some(([key]) => metrics[key] === null);
}

const CONTENT_SCOPE_LABELS: Record<HotKeyAPI.ContentTextScope, string> = {
  full: "完整原文",
  summary: "摘要",
  truncated: "已截断",
  media_only: "仅媒体",
};

const CONTENT_SCOPE_NOTICES: Partial<
  Record<HotKeyAPI.ContentTextScope, string>
> = {
  summary: "当前仅保存来源摘要，不代表完整原文。",
  truncated: "当前文本已截断，请结合原文入口核对。",
  media_only: "来源仅表明含媒体，未保存或理解媒体内容。",
};

export function contentScopeLabel(scope: HotKeyAPI.ContentTextScope): string {
  return CONTENT_SCOPE_LABELS[scope];
}

export function contentScopeNotice(
  scope: HotKeyAPI.ContentTextScope,
): string | null {
  return CONTENT_SCOPE_NOTICES[scope] ?? null;
}

export function contentOriginLabel(
  origin: HotKeyAPI.ContentTextOrigin,
): string {
  return origin === "source" ? "来源原文" : "机器提取";
}

export function truncationReasonLabel(
  reason: HotKeyAPI.ContentTruncationReason,
): string {
  return reason === "source_limit" ? "来源返回受限" : "采集器边界";
}

export function relationTypeLabel(
  relationType: HotKeyAPI.ContentRelationType,
): string {
  return relationType === "quote" ? "引用" : "转帖";
}
