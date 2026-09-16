// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Source Catalog GET /api/sources */
export async function listSources(options?: RequestOptions) {
  return request<API.SourceView[]>("/api/sources", {
    method: "GET",
    ...(options || {}),
  });
}

/** Query Preview POST /api/sources/query-preview */
export async function previewSourceQueries(
  body: API.QueryPreviewInput,
  options?: RequestOptions
) {
  return request<API.QueryPreview>("/api/sources/query-preview", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
