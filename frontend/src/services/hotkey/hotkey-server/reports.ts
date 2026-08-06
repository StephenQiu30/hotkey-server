// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List reports GET /api/v1/reports */
export async function getReports(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getReportsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.ReportResultHttpReportPageResponse>(
    "/api/v1/reports",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Get a report GET /api/v1/reports/${param0} */
export async function getReportsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getReportsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ReportResultHttpReportResponse>(
    `/api/v1/reports/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Build a report draft POST /api/v1/reports/${param0}/build */
export async function postReportsIdBuild(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postReportsIdBuildParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ReportResultHttpReportResponse>(
    `/api/v1/reports/${param0}/build`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Preview a report POST /api/v1/reports/${param0}/preview */
export async function postReportsIdPreview(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postReportsIdPreviewParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ReportResultHttpReportPreviewResponse>(
    `/api/v1/reports/${param0}/preview`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Publish a draft report POST /api/v1/reports/${param0}/publish */
export async function postReportsIdPublish(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postReportsIdPublishParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ReportResultHttpReportResponse>(
    `/api/v1/reports/${param0}/publish`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
