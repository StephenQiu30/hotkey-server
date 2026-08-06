// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List durable jobs GET /api/v1/operations/jobs */
export async function getOperationsJobs(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getOperationsJobsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.JobResultHttpJobPageResponse>(
    "/api/v1/operations/jobs",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Cancel a durable job POST /api/v1/operations/jobs/${param0}/cancel */
export async function postOperationsJobsIdCancel(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postOperationsJobsIdCancelParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.JobResultHttpJobResponse>(
    `/api/v1/operations/jobs/${param0}/cancel`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Retry a durable job POST /api/v1/operations/jobs/${param0}/retry */
export async function postOperationsJobsIdRetry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postOperationsJobsIdRetryParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.JobResultHttpJobResponse>(
    `/api/v1/operations/jobs/${param0}/retry`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Get runtime overview GET /api/v1/operations/overview */
export async function getOperationsOverview(options?: RequestOptions) {
  return request<HotKeyAPI.OverviewResultDomainRuntimeOverview>(
    "/api/v1/operations/overview",
    {
      method: "GET",
      ...(options || {}),
    }
  );
}
