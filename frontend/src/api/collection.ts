// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** List Collection Runs GET /api/collection-runs */
export async function listCollectionRuns(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listCollectionRunsParams,
  options?: RequestOptions
) {
  return request<API.CollectionRunPage>("/api/collection-runs", {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",
      ...params,
    },
    ...(options || {}),
  });
}

/** Get Collection Run GET /api/collection-runs/${param0} */
export async function getCollectionRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getCollectionRunParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.CollectionRunView>(`/api/collection-runs/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Create Collection Run POST /api/monitors/${param0}/runs */
export async function createCollectionRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createCollectionRunParams,
  body: API.CollectionRunRequest,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.CollectionRunView>(`/api/monitors/${param0}/runs`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
