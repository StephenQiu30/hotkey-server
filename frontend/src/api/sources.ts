// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Source Catalog GET /api/v1/sources */
export async function listSources(options?: RequestOptions) {
  return request<API.SourceView[]>("/api/v1/sources", {
    method: "GET",
    ...(options || {}),
  });
}

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
