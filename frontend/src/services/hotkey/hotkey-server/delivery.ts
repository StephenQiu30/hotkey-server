// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List the current user's report subscriptions GET /api/v1/report-subscriptions */
export async function getReportSubscriptions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getReportSubscriptionsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.DeliveryResultHttpSubscriptionPageResponse>(
    "/api/v1/report-subscriptions",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Create a report subscription POST /api/v1/report-subscriptions */
export async function postReportSubscriptions(
  body: HotKeyAPI.CreateSubscriptionRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.DeliveryResultHttpSubscriptionSecretResponse>(
    "/api/v1/report-subscriptions",
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

/** Get a report subscription GET /api/v1/report-subscriptions/${param0} */
export async function getReportSubscriptionsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getReportSubscriptionsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.DeliveryResultHttpSubscriptionResponse>(
    `/api/v1/report-subscriptions/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Delete a disabled report subscription DELETE /api/v1/report-subscriptions/${param0} */
export async function deleteReportSubscriptionsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.deleteReportSubscriptionsIdParams,
  body: HotKeyAPI.DeleteSubscriptionRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.DeliveryResultHttpSubscriptionResponse>(
    `/api/v1/report-subscriptions/${param0}`,
    {
      method: "DELETE",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Update a report subscription PATCH /api/v1/report-subscriptions/${param0} */
export async function patchReportSubscriptionsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.patchReportSubscriptionsIdParams,
  body: HotKeyAPI.UpdateSubscriptionRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.DeliveryResultHttpSubscriptionResponse>(
    `/api/v1/report-subscriptions/${param0}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Rotate a private RSS token POST /api/v1/report-subscriptions/${param0}/rss-token/rotate */
export async function postReportSubscriptionsIdRssTokenRotate(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postReportSubscriptionsIdRssTokenRotateParams,
  body: HotKeyAPI.RotateRSSTokenRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.DeliveryResultHttpSubscriptionSecretResponse>(
    `/api/v1/report-subscriptions/${param0}/rss-token/rotate`,
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
