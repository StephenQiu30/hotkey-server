// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Jobs GET /api/v1/jobs */
export async function listJobs(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listJobsParams,
  options?: RequestOptions
) {
  return request<API.JobPage>("/api/v1/jobs", {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",
      ...params,
    },
    ...(options || {}),
  });
}

/** Create Job POST /api/v1/jobs */
export async function createDiagnosticJob(
  body: API.DiagnosticInput,
  options?: RequestOptions
) {
  return request<API.JobView>("/api/v1/jobs", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Get Job GET /api/v1/jobs/${param0} */
export async function getJob(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getJobParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.JobView>(`/api/v1/jobs/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Cancel Job POST /api/v1/jobs/${param0}/cancel */
export async function cancelJob(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelJobParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.JobView>(`/api/v1/jobs/${param0}/cancel`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
