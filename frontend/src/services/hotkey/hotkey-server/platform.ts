// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get API capabilities GET /api/v1/capabilities */
export async function getCapabilities(options?: RequestOptions) {
  return request<HotKeyAPI.ResultHttpCapabilities>("/api/v1/capabilities", {
    method: "GET",
    ...(options || {}),
  });
}
