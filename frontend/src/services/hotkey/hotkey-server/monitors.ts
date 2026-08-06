// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List monitors GET /api/v1/monitors */
export async function getMonitors(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMonitorsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.MonitorResultHttpMonitorPageResponse>(
    "/api/v1/monitors",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Create a monitor draft POST /api/v1/monitors */
export async function postMonitors(
  body: HotKeyAPI.CreateMonitorRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.MonitorResultHttpMonitorResponse>(
    "/api/v1/monitors",
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

/** Get a monitor GET /api/v1/monitors/${param0} */
export async function getMonitorsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMonitorsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpMonitorResponse>(
    `/api/v1/monitors/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Delete an archived monitor DELETE /api/v1/monitors/${param0} */
export async function deleteMonitorsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.deleteMonitorsIdParams,
  body: HotKeyAPI.LifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpMonitorResponse>(
    `/api/v1/monitors/${param0}`,
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

/** Archive a monitor POST /api/v1/monitors/${param0}/archive */
export async function postMonitorsIdArchive(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdArchiveParams,
  body: HotKeyAPI.LifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpMonitorResponse>(
    `/api/v1/monitors/${param0}/archive`,
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

/** Replace a monitor draft PUT /api/v1/monitors/${param0}/draft */
export async function putMonitorsIdDraft(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.putMonitorsIdDraftParams,
  body: HotKeyAPI.ReplaceDraftRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpMonitorResponse>(
    `/api/v1/monitors/${param0}/draft`,
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

/** Add a pending AI rule candidate POST /api/v1/monitors/${param0}/draft/ai-candidates */
export async function postMonitorsIdDraftAiCandidates(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdDraftAiCandidatesParams,
  body: HotKeyAPI.AICandidateRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpMonitorRuleResponse>(
    `/api/v1/monitors/${param0}/draft/ai-candidates`,
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

/** Approve or reject an AI rule candidate POST /api/v1/monitors/${param0}/draft/rules/${param1}/approval */
export async function postMonitorsIdDraftRulesRuleIdApproval(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdDraftRulesRuleIdApprovalParams,
  body: HotKeyAPI.ApprovalRequest,
  options?: RequestOptions
) {
  const { id: param0, rule_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultInternalModulesMonitorTransportHttpEmptyResponse>(
    `/api/v1/monitors/${param0}/draft/rules/${param1}/approval`,
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

/** Pause a monitor POST /api/v1/monitors/${param0}/pause */
export async function postMonitorsIdPause(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdPauseParams,
  body: HotKeyAPI.LifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpMonitorResponse>(
    `/api/v1/monitors/${param0}/pause`,
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

/** Preview a monitor draft without persistence POST /api/v1/monitors/${param0}/preview */
export async function postMonitorsIdPreview(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdPreviewParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpPreviewResponse>(
    `/api/v1/monitors/${param0}/preview`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Publish a monitor draft POST /api/v1/monitors/${param0}/publish */
export async function postMonitorsIdPublish(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdPublishParams,
  body: HotKeyAPI.PublishRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpMonitorResponse>(
    `/api/v1/monitors/${param0}/publish`,
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

/** Restore a monitor POST /api/v1/monitors/${param0}/restore */
export async function postMonitorsIdRestore(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdRestoreParams,
  body: HotKeyAPI.LifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpMonitorResponse>(
    `/api/v1/monitors/${param0}/restore`,
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

/** Resume a monitor POST /api/v1/monitors/${param0}/resume */
export async function postMonitorsIdResume(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdResumeParams,
  body: HotKeyAPI.LifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorResultHttpMonitorResponse>(
    `/api/v1/monitors/${param0}/resume`,
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
