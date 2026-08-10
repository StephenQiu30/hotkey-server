// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List exact document matches for a monitor GET /api/v1/monitors/${param0}/document-matches */
export async function getMonitorsIdDocumentMatches(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMonitorsIdDocumentMatchesParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpDocumentMatchPageResponseDTO>(
    `/api/v1/monitors/${param0}/document-matches`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Append a document match review decision POST /api/v1/monitors/${param0}/document-matches/${param1}/overrides */
export async function postMonitorsIdDocumentMatchesMatchDecisionIdOverrides(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdDocumentMatchesMatchDecisionIdOverridesParams,
  body: HotKeyAPI.OverrideDocumentMatchRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, match_decision_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpOverrideDocumentMatchResponseDTO>(
    `/api/v1/monitors/${param0}/document-matches/${param1}/overrides`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}
