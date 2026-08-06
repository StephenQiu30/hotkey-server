// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List events GET /api/v1/events */
export async function getEvents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getEventsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.EventResultHttpEventPageResponse>("/api/v1/events", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** Get event GET /api/v1/events/${param0} */
export async function getEventsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getEventsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpEventResponse>(
    `/api/v1/events/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Save an event claim POST /api/v1/events/${param0}/claims */
export async function postEventsIdClaims(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postEventsIdClaimsParams,
  body: HotKeyAPI.ClaimRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpClaimResponse>(
    `/api/v1/events/${param0}/claims`,
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

/** List event contents GET /api/v1/events/${param0}/contents */
export async function getEventsIdContents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getEventsIdContentsParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpEventMemberPageResponse>(
    `/api/v1/events/${param0}/contents`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Lock an event member POST /api/v1/events/${param0}/contents/${param1}/lock */
export async function postEventsIdContentsContentIdLock(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postEventsIdContentsContentIdLockParams,
  body: HotKeyAPI.MemberLockRequest,
  options?: RequestOptions
) {
  const { id: param0, content_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpEventMemberResponse>(
    `/api/v1/events/${param0}/contents/${param1}/lock`,
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

/** Get latest event heat GET /api/v1/events/${param0}/heat */
export async function getEventsIdHeat(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getEventsIdHeatParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpHeatResponse>(
    `/api/v1/events/${param0}/heat`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Get verified event intelligence GET /api/v1/events/${param0}/intelligence */
export async function getEventsIdIntelligence(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getEventsIdIntelligenceParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpEventIntelligenceResponse>(
    `/api/v1/events/${param0}/intelligence`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Regenerate event entities and claims POST /api/v1/events/${param0}/intelligence/extract */
export async function postEventsIdIntelligenceExtract(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postEventsIdIntelligenceExtractParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpExtractionRegenerationResponse>(
    `/api/v1/events/${param0}/intelligence/extract`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Regenerate event summary POST /api/v1/events/${param0}/intelligence/summary/regenerate */
export async function postEventsIdIntelligenceSummaryRegenerate(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postEventsIdIntelligenceSummaryRegenerateParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpSummaryRegenerationResponse>(
    `/api/v1/events/${param0}/intelligence/summary/regenerate`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Change event lifecycle POST /api/v1/events/${param0}/lifecycle */
export async function postEventsIdLifecycle(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postEventsIdLifecycleParams,
  body: HotKeyAPI.LifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpEventResponse>(
    `/api/v1/events/${param0}/lifecycle`,
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

/** Merge events POST /api/v1/events/${param0}/merge */
export async function postEventsIdMerge(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postEventsIdMergeParams,
  body: HotKeyAPI.MergeRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpEventResponse>(
    `/api/v1/events/${param0}/merge`,
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

/** Split event POST /api/v1/events/${param0}/split */
export async function postEventsIdSplit(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postEventsIdSplitParams,
  body: HotKeyAPI.SplitRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpEventResponse>(
    `/api/v1/events/${param0}/split`,
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

/** List event updates GET /api/v1/events/${param0}/updates */
export async function getEventsIdUpdates(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getEventsIdUpdatesParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.EventResultHttpEventUpdatePageResponse>(
    `/api/v1/events/${param0}/updates`,
    {
      method: "GET",
      params: {
        // limit has a default value: 50
        limit: "50",
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}
