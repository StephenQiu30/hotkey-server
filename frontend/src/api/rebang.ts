// @ts-ignore
/* eslint-disable */
import request from "@/request";

/** 读取最新热榜快照 读取当前用户最近一次快照及与上次快照的排名变化。按排名游标分页。 GET /api/hotlists/${param0} */
export async function getHotlistSnapshot(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getHotlistSnapshotParams,
  options?: import("@/request").RequestOptions,
) {
  const { source_key: param0, ...queryParams } = params;
  return request<HotKeyAPI.HotlistSnapshotView>(`/api/hotlists/${param0}`, {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 列出已应用热榜来源 仅列出当前用户已应用的热榜来源及最近快照时间。读取不会访问 RSSHub。 GET /api/hotlists/sources */
export async function listHotlistSources(
  options?: import("@/request").RequestOptions,
) {
  return request<HotKeyAPI.PageViewHotlistSourceView_>(
    "/api/hotlists/sources",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}
