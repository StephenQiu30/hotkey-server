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

/** Advance the current user's durable notification read cursor POST /api/v1/notifications/read-receipts */
export async function postNotificationsReadReceipts(
  body: HotKeyAPI.RecordNotificationReadReceiptRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.NotificationResultHttpNotificationReadReceiptResponseDTO>(
    "/api/v1/notifications/read-receipts",
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

/** Upgrade to the authenticated notification WebSocket Request hotkey.notifications.v1, then send one authenticate frame containing token, after_id and optional monitor_id before business data is emitted. GET /api/v1/notifications/ws */
export async function getNotificationsWs(options?: RequestOptions) {
  return request<any>("/api/v1/notifications/ws", {
    method: "GET",
    ...(options || {}),
  });
}
