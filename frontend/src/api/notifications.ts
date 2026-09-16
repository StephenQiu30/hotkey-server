// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Notifications GET /api/notifications */
export async function listNotifications(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listNotificationsParams,
  options?: RequestOptions
) {
  return request<API.NotificationPage>("/api/notifications", {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",

      ...params,
    },
    ...(options || {}),
  });
}

/** Mark Notification Read POST /api/notifications/${param0}/read */
export async function markNotificationRead(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.markNotificationReadParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.NotificationView>(`/api/notifications/${param0}/read`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
