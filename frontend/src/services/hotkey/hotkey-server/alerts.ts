// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List alert threads GET /api/v1/alerts */
export async function getAlerts(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getAlertsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.AlertResultHttpAlertPageResponse>("/api/v1/alerts", {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",
      ...params,
    },
    ...(options || {}),
  });
}

/** Get an alert thread GET /api/v1/alerts/${param0} */
export async function getAlertsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getAlertsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.AlertResultHttpAlertDetailResponse>(
    `/api/v1/alerts/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Acknowledge an alert thread POST /api/v1/alerts/${param0}/acknowledge */
export async function postAlertsIdAcknowledge(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postAlertsIdAcknowledgeParams,
  body: HotKeyAPI.AlertActionRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.AlertResultHttpAlertThreadResponse>(
    `/api/v1/alerts/${param0}/acknowledge`,
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

/** Resolve an alert thread POST /api/v1/alerts/${param0}/resolve */
export async function postAlertsIdResolve(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postAlertsIdResolveParams,
  body: HotKeyAPI.AlertActionRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.AlertResultHttpAlertThreadResponse>(
    `/api/v1/alerts/${param0}/resolve`,
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

/** Suppress an alert thread POST /api/v1/alerts/${param0}/suppress */
export async function postAlertsIdSuppress(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postAlertsIdSuppressParams,
  body: HotKeyAPI.AlertActionRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.AlertResultHttpAlertThreadResponse>(
    `/api/v1/alerts/${param0}/suppress`,
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
