// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Search current PostgreSQL content, event and knowledge projections GET /api/v1/search */
export async function getSearch(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getSearchParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.SearchResultHttpSearchPageResponseDTO>(
    "/api/v1/search",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

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
