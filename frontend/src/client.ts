import createClient from "openapi-fetch";
import type { paths, components } from "./api.generated";
export type Monitor = components["schemas"]["MonitorView"];
export type Job = components["schemas"]["JobView"];
export type Source = components["schemas"]["SourceView"];
export const api = createClient<paths>({ credentials: "same-origin" });
api.use({
  onRequest({ request }) {
    if (!["GET", "HEAD", "OPTIONS"].includes(request.method)) {
      const csrf = document.cookie
        .split("; ")
        .find((v) => v.startsWith("hk_csrf="))
        ?.split("=")[1];
      if (csrf) request.headers.set("X-CSRF-Token", csrf);
    }
    return request;
  },
});
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
export function message(error: unknown): string {
  if (
    error &&
    typeof error === "object" &&
    "code" in error &&
    typeof error.code === "string"
  ) {
    return messages[error.code] ?? `请求失败：${error.code}`;
  }
  return "连接失败，请检查服务状态后重试。";
}
export function unwrap<T>(result: { data?: T; error?: unknown }): T {
  if (result.error || result.data === undefined)
    throw result.error ?? new Error("empty response");
  return result.data;
}
