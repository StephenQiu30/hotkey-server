// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List active content GET /api/v1/contents */
export async function getContents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getContentsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.ContentResultHttpContentPageResponse>(
    "/api/v1/contents",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Get active content GET /api/v1/contents/${param0} */
export async function getContentsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getContentsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpContentResponse>(
    `/api/v1/contents/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Delete active content Soft-deletes one fetched Content and removes its archived evidence when available. DELETE /api/v1/contents/${param0} */
export async function deleteContentsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.deleteContentsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultInternalModulesIngestionTransportHttpEmptyResponse>(
    `/api/v1/contents/${param0}`,
    {
      method: "DELETE",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Get captured content Markdown document Returns not_captured as a successful empty state when no authorized Markdown asset exists. GET /api/v1/contents/${param0}/document */
export async function getContentsIdDocument(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getContentsIdDocumentParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpContentDocumentResponse>(
    `/api/v1/contents/${param0}/document`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
