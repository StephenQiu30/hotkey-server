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

const VISIBILITY_STATUS_LABELS: Record<
  HotKeyAPI.ContentVisibilityStatus,
  string
> = {
  visible: "来源可见",
  deleted: "来源已删除",
  restricted: "来源访问受限",
  transient_failure: "来源暂时不可达",
  unknown: "来源状态未知",
};

const VISIBILITY_STATUS_NOTICES: Record<
  HotKeyAPI.ContentVisibilityStatus,
  string
> = {
  visible: "最近一次来源观察取得了可保存的作品资料。",
  deleted: "来源明确表明作品已删除；页面仅保留仍在本地保留期内的最后成功资料。",
  restricted: "当前权限或认证不足，不能据此判断作品是否仍公开可见。",
  transient_failure: "来源暂时不可达，保留最后成功资料；这不表示作品已删除。",
  unknown: "来源状态无法确认，不能据此认定删除。",
};

const VISIBILITY_BASIS_LABELS: Record<
  HotKeyAPI.ContentVisibilityBasis,
  string
> = {
  content_returned: "取得作品资料",
  source_tombstone: "来源删除标识",
  http_gone: "来源明确永久不可用",
  access_denied: "访问被拒绝",
  authentication_required: "需要重新认证",
  not_found: "未找到当前表示",
  timeout: "来源响应超时",
  rate_limited: "来源限流",
  upstream_error: "来源服务失败",
  protocol_error: "来源响应无法判定",
};

export function visibilityStatusLabel(
  status: HotKeyAPI.ContentVisibilityStatus,
): string {
  return VISIBILITY_STATUS_LABELS[status];
}

export function visibilityStatusNotice(
  status: HotKeyAPI.ContentVisibilityStatus,
): string {
  return VISIBILITY_STATUS_NOTICES[status];
}

export function visibilityBasisLabel(
  basis: HotKeyAPI.ContentVisibilityBasis,
): string {
  return VISIBILITY_BASIS_LABELS[basis];
}
