// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get an exact document-version citation GET /api/v1/document-versions/${param0}/citation */
export async function getDocumentVersionsIdCitation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getDocumentVersionsIdCitationParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpCitationResponseDTO>(
    `/api/v1/document-versions/${param0}/citation`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Get an exact document-version Markdown projection GET /api/v1/document-versions/${param0}/document */
export async function getDocumentVersionsIdDocument(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getDocumentVersionsIdDocumentParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpVersionedDocumentResponseDTO>(
    `/api/v1/document-versions/${param0}/document`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
