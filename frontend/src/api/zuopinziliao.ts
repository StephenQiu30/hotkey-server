// @ts-ignore
/* eslint-disable */
import request from "@/request";

/** 列出作品资料 按当前 owner 列出具有可读观察的作品; 读取不会触发来源请求。 GET /api/contents */
export async function listContentRecords(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.listContentRecordsParams,
  options?: import("@/request").RequestOptions,
) {
  return request<HotKeyAPI.PageViewContentRecordSummaryView_>("/api/contents", {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",
      ...params,
    },
    ...(options || {}),
  });
}

/** 读取作品资料 读取当前 owner 的作品身份、最新可读观察与发现依据; 不隐式刷新。 GET /api/contents/${param0} */
export async function getContentRecord(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getContentRecordParams,
  options?: import("@/request").RequestOptions,
) {
  const { content_id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentRecordDetailView>(`/api/contents/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}
