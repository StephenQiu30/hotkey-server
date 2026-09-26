// @ts-ignore
/* eslint-disable */
import request from "@/request";

/** 列出日报 仅列出当前 owner 的定稿报告; 每个主题及时间窗只返回最新版本。日期按 Asia/Shanghai 自然日筛选。 GET /api/v1/reports */
export async function listReports(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.listReportsParams,
  options?: import("@/request").RequestOptions,
) {
  return request<HotKeyAPI.PageViewReportSummaryView_>("/api/v1/reports", {
    method: "GET",
    params: {
      // kind has a default value: daily
      kind: "daily",

      // limit has a default value: 20
      limit: "20",
      ...params,
    },
    ...(options || {}),
  });
}

/** 读取日报详情 按当前 owner 读取指定定稿版本的 Markdown、窗口、截止时间和原帖引用。 GET /api/v1/reports/${param0} */
export async function getReport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getReportParams,
  options?: import("@/request").RequestOptions,
) {
  const { report_id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ReportDetailView>(`/api/v1/reports/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
