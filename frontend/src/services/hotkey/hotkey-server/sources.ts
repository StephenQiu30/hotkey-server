// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List source connections GET /api/v1/source-connections */
export async function getSourceConnections(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getSourceConnectionsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.SourceResultHttpSourceReadPageResponse>(
    "/api/v1/source-connections",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Create a source connection POST /api/v1/source-connections */
export async function postSourceConnections(
  body: HotKeyAPI.CreateSourceRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.SourceResultHttpManagementSourceResponse>(
    "/api/v1/source-connections",
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

/** Get a source connection GET /api/v1/source-connections/${param0} */
export async function getSourceConnectionsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getSourceConnectionsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpSourceReadResponse>(
    `/api/v1/source-connections/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Update a source connection PATCH /api/v1/source-connections/${param0} */
export async function patchSourceConnectionsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.patchSourceConnectionsIdParams,
  body: HotKeyAPI.UpdateSourceRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpManagementSourceResponse>(
    `/api/v1/source-connections/${param0}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Archive a source connection POST /api/v1/source-connections/${param0}/archive */
export async function postSourceConnectionsIdArchive(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postSourceConnectionsIdArchiveParams,
  body: HotKeyAPI.SourceLifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpManagementSourceResponse>(
    `/api/v1/source-connections/${param0}/archive`,
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

/** Disable a source connection POST /api/v1/source-connections/${param0}/disable */
export async function postSourceConnectionsIdDisable(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postSourceConnectionsIdDisableParams,
  body: HotKeyAPI.SourceLifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpManagementSourceResponse>(
    `/api/v1/source-connections/${param0}/disable`,
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

/** Enable a source connection POST /api/v1/source-connections/${param0}/enable */
export async function postSourceConnectionsIdEnable(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postSourceConnectionsIdEnableParams,
  body: HotKeyAPI.SourceLifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpManagementSourceResponse>(
    `/api/v1/source-connections/${param0}/enable`,
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

/** Probe source connection health POST /api/v1/source-connections/${param0}/health */
export async function postSourceConnectionsIdHealth(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postSourceConnectionsIdHealthParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.CollectionResultHttpSourceHealthResponse>(
    `/api/v1/source-connections/${param0}/health`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Restore a source connection POST /api/v1/source-connections/${param0}/restore */
export async function postSourceConnectionsIdRestore(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postSourceConnectionsIdRestoreParams,
  body: HotKeyAPI.SourceLifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpManagementSourceResponse>(
    `/api/v1/source-connections/${param0}/restore`,
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
