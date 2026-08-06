// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List collection runs GET /api/v1/collection-runs */
export async function getCollectionRuns(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getCollectionRunsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.CollectionResultHttpCollectionRunPageResponse>(
    "/api/v1/collection-runs",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Requeue a failed collection run POST /api/v1/collection-runs/${param0}/retry */
export async function postCollectionRunsIdRetry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postCollectionRunsIdRetryParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.CollectionResultHttpCollectionRunResponse>(
    `/api/v1/collection-runs/${param0}/retry`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
