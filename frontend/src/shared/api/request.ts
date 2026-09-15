import axios, { type AxiosRequestConfig } from "axios";

export type RequestOptions = AxiosRequestConfig & {
  requestType?: "form";
};

type ApiError = {
  code?: string;
  message?: string;
  request_id?: string;
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
