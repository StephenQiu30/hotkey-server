// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List current user's monitor notifications after a durable cursor GET /api/v1/notifications */
export async function getNotifications(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getNotificationsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.NotificationResultHttpUserNotificationPageResponseDTO>(
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

/** Get Web Push capability GET /api/v1/notifications/push-capability */
export async function getNotificationsPushCapability(options?: RequestOptions) {
  return request<HotKeyAPI.NotificationResultHttpPushCapabilityResponseDTO>(
    "/api/v1/notifications/push-capability",
    {
      method: "GET",
      ...(options || {}),
    }
  );
}

/** List current user's Web Push devices GET /api/v1/notifications/push-subscriptions */
export async function getNotificationsPushSubscriptions(
  options?: RequestOptions
) {
  return request<HotKeyAPI.NotificationResultHttpPushSubscriptionListResponseDTO>(
    "/api/v1/notifications/push-subscriptions",
    {
      method: "GET",
      ...(options || {}),
    }
  );
}

/** Register a Web Push device POST /api/v1/notifications/push-subscriptions */
export async function postNotificationsPushSubscriptions(
  body: HotKeyAPI.RegisterPushSubscriptionRequestDTO,
  options?: RequestOptions
) {
  return request<HotKeyAPI.NotificationResultHttpPushSubscriptionResponseDTO>(
    "/api/v1/notifications/push-subscriptions",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    }
  );
}

/** Update a Web Push device PUT /api/v1/notifications/push-subscriptions/${param0} */
export async function putNotificationsPushSubscriptionsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.putNotificationsPushSubscriptionsIdParams,
  body: HotKeyAPI.UpdatePushSubscriptionRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.NotificationResultHttpPushSubscriptionResponseDTO>(
    `/api/v1/notifications/push-subscriptions/${param0}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Disable a Web Push device DELETE /api/v1/notifications/push-subscriptions/${param0} */
export async function deleteNotificationsPushSubscriptionsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.deleteNotificationsPushSubscriptionsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.NotificationResultHttpPushSubscriptionResponseDTO>(
    `/api/v1/notifications/push-subscriptions/${param0}`,
    {
      method: "DELETE",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Stream current user's monitor notifications with durable replay GET /api/v1/notifications/stream */
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
