import { APIErrorCode } from "./domainEnums";

const API_ERROR_MESSAGES: Readonly<Record<APIErrorCode, string>> = {
  [APIErrorCode.InvalidRequest]: "请求参数无效，请检查后重试",
  [APIErrorCode.ValidationFailed]: "提交内容未通过校验，请检查后重试",
  [APIErrorCode.VersionConflict]: "当前数据已发生变化，请刷新后重试",
  [APIErrorCode.NotFound]: "请求的内容不存在或已被删除",
  [APIErrorCode.RateLimited]: "操作过于频繁，请稍后重试",
  [APIErrorCode.Unauthorized]: "请先登录后再继续",
  [APIErrorCode.Forbidden]: "当前账号没有执行此操作的权限",
  [APIErrorCode.InvalidCredentials]: "邮箱或密码错误",
  [APIErrorCode.SessionExpired]: "登录已过期，请重新登录",
  [APIErrorCode.InvalidVerification]: "验证码或验证凭证无效",
  [APIErrorCode.LastAdminRequired]: "必须保留至少一名启用的管理员",
  [APIErrorCode.InvalidMonitorState]: "当前监控状态不支持此操作",
  [APIErrorCode.MonitorVersionConflict]: "监控已被更新，请刷新后重试",
  [APIErrorCode.InvalidMonitorConfig]: "监控配置无效，请检查后重试",
  [APIErrorCode.MonitorDraftMissing]: "当前监控没有可用的草稿",
  [APIErrorCode.MonitorNameConflict]: "监控名称已存在",
  [APIErrorCode.InvalidSourceConfig]: "来源配置无效，请检查后重试",
  [APIErrorCode.SourceConnectionMissing]: "请先配置来源连接",
  [APIErrorCode.UnsupportedSourceType]: "暂不支持此来源类型",
  [APIErrorCode.SourceUnavailable]: "来源连接当前不可用",
  [APIErrorCode.CollectionJobNotFound]: "采集任务不存在",
  [APIErrorCode.CollectionJobConflict]: "采集任务状态已发生变化，请刷新后重试",
  [APIErrorCode.InvalidCollectionRequest]: "采集请求无效，请检查后重试",
  [APIErrorCode.InvalidAIModelConfig]: "AI 模型配置无效",
  [APIErrorCode.AIModelUnavailable]: "AI 模型暂时不可用，请稍后重试",
  [APIErrorCode.AIQuotaExhausted]: "AI 使用额度已耗尽",
  [APIErrorCode.AIRateLimited]: "AI 服务请求过于频繁，请稍后重试",
  [APIErrorCode.AIServiceError]: "AI 服务暂时异常，请稍后重试",
  [APIErrorCode.AIServiceTimeout]: "AI 服务响应超时，请稍后重试",
  [APIErrorCode.AIInvalidResponse]: "AI 返回内容无效，请重新生成",
  [APIErrorCode.AIJobRunning]: "AI 任务正在运行，请稍后查看",
  [APIErrorCode.AIInvalidVector]: "AI 向量数据无效",
  [APIErrorCode.AILeaseExpired]: "AI 任务租约已过期，请重新运行",
  [APIErrorCode.InternalServerError]: "服务器内部错误，请稍后重试",
  [APIErrorCode.ServiceUnavailable]: "服务暂时不可用，请稍后重试",
  [APIErrorCode.UpstreamServiceError]: "上游服务异常，请稍后重试",
  [APIErrorCode.RequestTimeout]: "请求处理超时，请稍后重试",
};

export function getUserFacingAPIErrorMessage(
  apiCode: number | undefined,
  fallback: string | undefined,
): string {
  if (apiCode !== undefined && apiCode in API_ERROR_MESSAGES) {
    return API_ERROR_MESSAGES[apiCode as APIErrorCode];
  }

  return fallback?.trim() || "操作失败，请稍后重试";
}
