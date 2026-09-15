// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Events GET /api/v1/events */
export async function listEvents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listEventsParams,
  options?: RequestOptions
) {
  return request<API.EventPage>("/api/v1/events", {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",
      ...params,
    },
    ...(options || {}),
  });
}

/** Create Event POST /api/v1/events */
export async function createEvent(
  body: API.EventInput,
  options?: RequestOptions
) {
  return request<API.EventView>("/api/v1/events", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Get Event GET /api/v1/events/${param0} */
export async function getEvent(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getEventParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.EventView>(`/api/v1/events/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Add Event Member POST /api/v1/events/${param0}/members */
export async function addEventMember(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.addEventMemberParams,
  body: API.EventMemberInput,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.EventView>(`/api/v1/events/${param0}/members`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Remove Event Member DELETE /api/v1/events/${param0}/members/${param1} */
export async function removeEventMember(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.removeEventMemberParams,
  options?: RequestOptions
) {
  const { identity: param0, content_id: param1, ...queryParams } = params;
  return request<API.EventView>(`/api/v1/events/${param0}/members/${param1}`, {
    method: "DELETE",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Merge Event POST /api/v1/events/${param0}/merge */
export async function mergeEvent(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.mergeEventParams,
  body: API.EventMergeInput,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.EventView>(`/api/v1/events/${param0}/merge`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Event Revisions GET /api/v1/events/${param0}/revisions */
export async function listEventRevisions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listEventRevisionsParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.EventRevisionView[]>(
    `/api/v1/events/${param0}/revisions`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Split Event POST /api/v1/events/${param0}/split */
export async function splitEvent(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.splitEventParams,
  body: API.EventSplitInput,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.EventView>(`/api/v1/events/${param0}/split`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** Event Trends GET /api/v1/events/${param0}/trends */
export async function getEventTrends(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getEventTrendsParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.EventTrendView>(`/api/v1/events/${param0}/trends`, {
    method: "GET",
    params: {
      // bucket_hours has a default value: 24
      bucket_hours: "24",
      ...queryParams,
    },
    ...(options || {}),
  });
}
