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
