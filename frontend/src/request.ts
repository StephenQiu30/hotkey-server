import axios, { type AxiosError, type AxiosRequestConfig } from "axios";

export type RequestOptions = AxiosRequestConfig & {
  requestType?: "form";
};

type ErrorResponse = {
  code?: string;
  detail?: unknown;
  message?: string;
};

export class ApiRequestError extends Error {
  readonly code?: string;
  readonly details?: unknown;
  readonly requestId?: string;
  readonly status?: number;

  constructor(error: AxiosError<ErrorResponse>) {
    const data = error.response?.data;
    const message =
      data?.message ??
      (typeof data?.detail === "string" ? data.detail : error.message);

    super(message);
    this.name = "ApiRequestError";
    this.code = data?.code ?? error.code;
    this.details = data?.detail;
    this.requestId = error.response?.headers["x-request-id"];
    this.status = error.response?.status;
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
  (error: unknown) =>
    Promise.reject(
      axios.isAxiosError<ErrorResponse>(error)
        ? new ApiRequestError(error)
        : error,
    ),
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

  return response.data;
}

export default request;
