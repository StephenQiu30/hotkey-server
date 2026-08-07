// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List notification events after a durable cursor GET /api/v1/notifications */
export async function getNotifications(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getNotificationsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.NotificationResultHttpNotificationPageResponse>(
    "/api/v1/notifications",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Stream notification events after a durable cursor GET /api/v1/notifications/stream */
export async function getNotificationsStream(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getNotificationsStreamParams,
  options?: RequestOptions
) {
  return request<string>("/api/v1/notifications/stream", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}
