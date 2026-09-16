// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Get Analysis Run GET /api/analysis-runs/${param0} */
export async function getAnalysisRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAnalysisRunParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.AnalysisRunView>(`/api/analysis-runs/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Recompute Analysis Run POST /api/analysis-runs/${param0}/recompute */
export async function recomputeAnalysisRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.recomputeAnalysisRunParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.AnalysisRunView>(
    `/api/analysis-runs/${param0}/recompute`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Label Analysis Sample PUT /api/analysis-runs/${param0}/samples/${param1}/label */
export async function labelAnalysisSample(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.labelAnalysisSampleParams,
  body: API.AnalysisLabelInput,
  options?: RequestOptions
) {
  const { identity: param0, sample_id: param1, ...queryParams } = params;
  return request<API.AnalysisRunView>(
    `/api/analysis-runs/${param0}/samples/${param1}/label`,
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

/** List Event Analysis Runs GET /api/events/${param0}/analysis-runs */
export async function listEventAnalysisRuns(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listEventAnalysisRunsParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.AnalysisRunPage>(`/api/events/${param0}/analysis-runs`, {
    method: "GET",
    params: {
      // limit has a default value: 5
      limit: "5",
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** Create Event Analysis Run POST /api/events/${param0}/analysis-runs */
export async function createEventAnalysisRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createEventAnalysisRunParams,
  body: API.AnalysisRunInput,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.AnalysisRunView>(`/api/events/${param0}/analysis-runs`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
