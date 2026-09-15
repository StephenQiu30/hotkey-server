import axios, { type AxiosRequestConfig } from "axios";

export type RequestOptions = AxiosRequestConfig & {
  requestType?: "form";
};

type ApiError = {
  code?: string;
  message?: string;
  request_id?: string;
};

const messages: Record<string, string> = {
  authentication_required: "会话已失效，请重新登录。",
  invalid_credentials: "用户名或密码不正确。",
  login_throttled: "尝试次数过多，请在 15 分钟后重试。",
  version_conflict: "草稿已被更新，请重新加载后编辑。",
  monitor_not_editable: "运行中的配置不可覆盖，请先暂停监控。",
  monitor_state_conflict: "监控状态已经变化，请刷新后重试。",
  source_not_eligible: "所选来源尚未通过用途权限和采集连接检查。",
  monitor_budget_insufficient: "每日请求上限不足以覆盖当前查询和检查周期。",
  csrf_invalid: "会话校验失败，请重新登录。",
  validation_failed: "请检查输入的长度、来源和关键词。",
  database_unavailable: "数据库暂时不可用，请稍后重试。",
  origin_forbidden: "访问地址未被允许，请检查服务配置。",
  analysis_sample_empty: "当前事件在所选范围没有可分析评论。",
  analysis_citation_outside_sample: "引用必须来自该条冻结样本的上下文。",
  analysis_label_exists: "该样本已经有不同的人工标签。",
};

const client = axios.create({
  baseURL: "/",
  withCredentials: true,
  withXSRFToken: true,
  xsrfCookieName: "hk_csrf",
  xsrfHeaderName: "X-CSRF-Token",
  paramsSerializer: { indexes: null },
});

export async function request<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { requestType: _requestType, ...config } = options;
  try {
    const response = await client.request<T>({ url: path, ...config });
    return response.data;
  } catch (error) {
    if (axios.isAxiosError<ApiError>(error) && error.response?.data) {
      throw error.response.data;
    }
    throw error;
  }
}

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
