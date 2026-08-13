// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List persisted hotspots GET /api/v1/hotspots */
export async function getHotspots(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getHotspotsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.ContentResultHttpHotspotPageResponse>(
    "/api/v1/hotspots",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}
