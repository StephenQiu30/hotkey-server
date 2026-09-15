// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Monitors GET /api/v1/monitors */
export async function listMonitors(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMonitorsParams,
  options?: RequestOptions
) {
  return request<API.MonitorPage>("/api/v1/monitors", {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",
      ...params,
    },
    ...(options || {}),
  });
}

/** Create Monitor POST /api/v1/monitors */
export async function createMonitor(
  body: API.MonitorInput,
  options?: RequestOptions
) {
  return request<API.MonitorView>("/api/v1/monitors", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Update Monitor PATCH /api/v1/monitors/${param0} */
export async function updateMonitor(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateMonitorParams,
  body: API.MonitorUpdate,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.MonitorView>(`/api/v1/monitors/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Activate Monitor POST /api/v1/monitors/${param0}/activate */
export async function activateMonitor(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.activateMonitorParams,
  body: API.MonitorStateChange,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.MonitorView>(`/api/v1/monitors/${param0}/activate`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Pause Monitor POST /api/v1/monitors/${param0}/pause */
export async function pauseMonitor(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.pauseMonitorParams,
  body: API.MonitorStateChange,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.MonitorView>(`/api/v1/monitors/${param0}/pause`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
