// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get the current monitor intent draft GET /api/v1/monitors/${param0}/draft */
export async function getMonitorsIdDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMonitorsIdDraftParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpIntentDraftResponseDTO>(
    `/api/v1/monitors/${param0}/draft`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Review a monitor intent expansion candidate POST /api/v1/monitors/${param0}/draft/expansion-candidates/${param1}/decision */
export async function postMonitorsIdDraftExpansionCandidatesCandidateIdDecision(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdDraftExpansionCandidatesCandidateIdDecisionParams,
  body: HotKeyAPI.ReviewIntentExpansionCandidateRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, candidate_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpIntentDraftResponseDTO>(
    `/api/v1/monitors/${param0}/draft/expansion-candidates/${param1}/decision`,
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

/** Submit a monitor intent expansion run POST /api/v1/monitors/${param0}/draft/expansion-runs */
export async function postMonitorsIdDraftExpansionRuns(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdDraftExpansionRunsParams,
  body: HotKeyAPI.SubmitIntentExpansionRunRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/v1/monitors/${param0}/draft/expansion-runs`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Get a monitor intent expansion run GET /api/v1/monitors/${param0}/draft/expansion-runs/${param1} */
export async function getMonitorsIdDraftExpansionRunsRunId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMonitorsIdDraftExpansionRunsRunIdParams,
  options?: RequestOptions
) {
  const { id: param0, run_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpIntentExpansionRunStatusResponseDTO>(
    `/api/v1/monitors/${param0}/draft/expansion-runs/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Initialize or replace the current monitor intent draft PUT /api/v1/monitors/${param0}/draft/intent */
export async function putMonitorsIdDraftIntent(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.putMonitorsIdDraftIntentParams,
  body: HotKeyAPI.ReplaceIntentDraftRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpIntentDraftResponseDTO>(
    `/api/v1/monitors/${param0}/draft/intent`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Submit a monitor intent preview run POST /api/v1/monitors/${param0}/draft/preview-runs */
export async function postMonitorsIdDraftPreviewRuns(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdDraftPreviewRunsParams,
  body: HotKeyAPI.SubmitIntentPreviewRunRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<any>(`/api/v1/monitors/${param0}/draft/preview-runs`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Get a monitor intent preview run GET /api/v1/monitors/${param0}/draft/preview-runs/${param1} */
export async function getMonitorsIdDraftPreviewRunsRunId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMonitorsIdDraftPreviewRunsRunIdParams,
  options?: RequestOptions
) {
  const { id: param0, run_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpIntentPreviewRunStatusResponseDTO>(
    `/api/v1/monitors/${param0}/draft/preview-runs/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
