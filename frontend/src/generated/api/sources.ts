// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../../shared/api/request";

/** Query Preview POST /api/v1/sources/bluesky/query-preview */
export async function previewBlueskyQuery(
  body: API.SearchInput,
  options?: RequestOptions
) {
  return request<API.QueryPreview>("/api/v1/sources/bluesky/query-preview", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
