import axios, {
  type AxiosError,
  type AxiosRequestConfig,
  type AxiosResponseHeaders,
  type RawAxiosResponseHeaders,
} from "axios";

export type RequestOptions = AxiosRequestConfig & {
  requestType?: "form";
};

export type ApiRequestErrorKind =
  "http" | "network" | "timeout" | "cancelled" | "protocol";

type ApiRequestErrorOptions = {
  code?: string;
  details?: HotKeyAPI.ValidationErrorItem[];
  kind: ApiRequestErrorKind;
  message: string;
  requestId?: string;
  status?: number;
};

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isRequestId(value: unknown): value is string {
  return typeof value === "string" && UUID_PATTERN.test(value);
}

function isValidationDetail(
  value: unknown,
): value is HotKeyAPI.ValidationErrorItem {
  if (!isRecord(value) || !Array.isArray(value.location)) {
    return false;
  }

  return (
    value.location.every(
      (part) => typeof part === "string" || Number.isInteger(part),
    ) &&
    typeof value.message === "string" &&
    typeof value.type === "string"
  );
}

function isErrorView(value: unknown): value is HotKeyAPI.ErrorView {
  if (!isRecord(value)) {
    return false;
  }

  return (
    typeof value.code === "string" &&
    typeof value.message === "string" &&
    isRequestId(value.request_id) &&
    (value.details === undefined ||
      value.details === null ||
      (Array.isArray(value.details) && value.details.every(isValidationDetail)))
  );
}

function getHeader(
  headers: AxiosResponseHeaders | Partial<RawAxiosResponseHeaders> | undefined,
  name: string,
): unknown {
  if (!headers) {
    return undefined;
  }
  if ("get" in headers && typeof headers.get === "function") {
    return headers.get(name);
  }
  return headers[name];
}

function classifyTransportError(error: AxiosError<unknown>): ApiRequestError {
  if (error.code === "ERR_CANCELED" || axios.isCancel(error)) {
    return new ApiRequestError({
      kind: "cancelled",
      message: "请求已取消",
    });
  }

  if (error.code === "ECONNABORTED" || error.code === "ETIMEDOUT") {
    return new ApiRequestError({
      kind: "timeout",
      message: "请求超时，服务端可能仍在处理",
    });
  }

  if (!error.response) {
    return new ApiRequestError({
      kind: "network",
      message: "无法连接服务",
    });
  }

  const data = error.response.data;
  const status = error.response.status;
  if (!isErrorView(data)) {
    return new ApiRequestError({
      kind: "protocol",
      message: "服务返回了无法识别的错误响应",
      status,
    });
  }

  const headerRequestId = getHeader(error.response.headers, "x-request-id");
  return new ApiRequestError({
    code: data.code,
    details: data.details ?? undefined,
    kind: "http",
    message: data.message,
    requestId: isRequestId(headerRequestId) ? headerRequestId : data.request_id,
    status,
  });
}

export class ApiRequestError extends Error {
  readonly code?: string;
  readonly details?: HotKeyAPI.ValidationErrorItem[];
  readonly kind: ApiRequestErrorKind;
  readonly requestId?: string;
  readonly status?: number;

  constructor(options: ApiRequestErrorOptions) {
    super(options.message);
    this.name = "ApiRequestError";
    this.code = options.code;
    this.details = options.details;
    this.kind = options.kind;
    this.requestId = options.requestId;
    this.status = options.status;
  }
}

const client = axios.create({
  baseURL: "/",
  timeout: 15_000,
  withCredentials: true,
  headers: {
    Accept: "application/json",
  },
});

client.interceptors.response.use(
  (response) => response,
  (error: unknown) => {
    if (axios.isAxiosError<unknown>(error)) {
      return Promise.reject(classifyTransportError(error));
    }
    return Promise.reject(
      new ApiRequestError({
        kind: "protocol",
        message: "请求处理失败",
      }),
    );
  },
);

export async function request<T>(
  url: string,
  options: RequestOptions = {},
): Promise<T> {
  const config = { ...options };
  delete config.requestType;
  const response = await client.request<T>({
    ...config,
    url,
  });

  if (response.status === 204) {
    return undefined as T;
  }
  return response.data;
}

export default request;
