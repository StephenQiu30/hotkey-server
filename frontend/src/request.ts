import axios, { type AxiosRequestConfig } from "axios";

export type RequestOptions = AxiosRequestConfig & {
  requestType?: "form";
};

const http = axios.create({
  baseURL: "/",
  timeout: 15_000,
  withCredentials: true,
  headers: {
    Accept: "application/json",
  },
});

export default async function request<T>(
  url: string,
  options: RequestOptions = {},
): Promise<T> {
  const config = { ...options };
  delete config.requestType;
  const response = await http.request<T>({
    ...config,
    url,
  });

  return response.data;
}
