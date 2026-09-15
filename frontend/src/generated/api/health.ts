// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../../shared/api/request";

/** Live GET /health/live */
export async function healthLive(options?: RequestOptions) {
  return request<API.HealthView>("/health/live", {
    method: "GET",
    ...(options || {}),
  });
}

/** Ready GET /health/ready */
export async function healthReady(options?: RequestOptions) {
  return request<API.HealthView>("/health/ready", {
    method: "GET",
    ...(options || {}),
  });
}
