// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Search configured hotspot sources POST /api/v1/search */
export async function postSearch(
  body: HotKeyAPI.InstantSearchRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.SourceResultHttpInstantSearchResponse>(
    "/api/v1/search",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    }
  );
}
