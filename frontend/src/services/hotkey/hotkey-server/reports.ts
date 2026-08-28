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

/** Create or refresh a report draft POST /api/v1/reports */
export async function postReports(
  body: HotKeyAPI.CreateReportRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.ReportResultHttpReportResponse>("/api/v1/reports", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
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

/** Approve a report revision POST /api/v1/reports/${param0}/approve */
export async function postReportsIdApprove(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postReportsIdApproveParams,
  body: HotKeyAPI.ReportRevisionLifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ReportResultHttpReportResponse>(
    `/api/v1/reports/${param0}/approve`,
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

/** Reject a report revision POST /api/v1/reports/${param0}/reject */
export async function postReportsIdReject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postReportsIdRejectParams,
  body: HotKeyAPI.ReportRevisionLifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ReportResultHttpReportResponse>(
    `/api/v1/reports/${param0}/reject`,
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

/** Submit a report revision for approval POST /api/v1/reports/${param0}/submit */
export async function postReportsIdSubmit(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postReportsIdSubmitParams,
  body: HotKeyAPI.ReportRevisionLifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ReportResultHttpReportResponse>(
    `/api/v1/reports/${param0}/submit`,
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
