// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List explainable Radar events GET /api/v1/radar/events */
export async function getRadarEvents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getRadarEventsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.EventResultHttpRadarPageResponse>(
    "/api/v1/radar/events",
    {
      method: "GET",
      params: {
        // window has a default value: 24h
        window: "24h",

        // sort has a default value: momentum
        sort: "momentum",
        // limit has a default value: 50
        limit: "50",
        ...params,
      },
      ...(options || {}),
    }
  );
}
