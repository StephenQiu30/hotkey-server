const messages: Record<string, string> = {
  authentication_required: "会话已失效，请重新登录。",
  invalid_credentials: "用户名或密码不正确。",
  login_throttled: "尝试次数过多，请在 15 分钟后重试。",
  version_conflict: "草稿已被更新，请重新加载后编辑。",
  csrf_invalid: "会话校验失败，请重新登录。",
  validation_failed: "请检查输入的长度、来源和关键词。",
  database_unavailable: "数据库暂时不可用，请稍后重试。",
  origin_forbidden: "访问地址未被允许，请检查服务配置。",
};

export function errorCode(error: unknown): string | undefined {
  return error &&
    typeof error === "object" &&
    "code" in error &&
    typeof error.code === "string"
    ? error.code
    : undefined;
}

export function message(error: unknown): string {
  const code = errorCode(error);
  if (code) return messages[code] ?? `请求失败：${code}`;
  return "连接失败，请检查服务状态后重试。";
}
