// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Get safe source endpoint capability GET /api/v1/source-endpoints/${param0}/capabilities */
export async function getSourceEndpointsIdCapabilities(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getSourceEndpointsIdCapabilitiesParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpSourceEndpointCapabilityResponseDTO>(
    `/api/v1/source-endpoints/${param0}/capabilities`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** List source endpoint rights decision batches GET /api/v1/source-endpoints/${param0}/rights-decision-batches */
export async function getSourceEndpointsIdRightsDecisionBatches(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getSourceEndpointsIdRightsDecisionBatchesParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpRightsDecisionBatchPageResponseDTO>(
    `/api/v1/source-endpoints/${param0}/rights-decision-batches`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Record an atomic source rights decision batch POST /api/v1/source-endpoints/${param0}/rights-decision-batches */
export async function postSourceEndpointsIdRightsDecisionBatches(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postSourceEndpointsIdRightsDecisionBatchesParams,
  body: HotKeyAPI.RecordRightsDecisionBatchRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpRecordRightsDecisionBatchResponseDTO>(
    `/api/v1/source-endpoints/${param0}/rights-decision-batches`,
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

/** Get one source rights decision GET /api/v1/source-endpoints/${param0}/rights-decisions/${param1} */
export async function getSourceEndpointsIdRightsDecisionsDecisionId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getSourceEndpointsIdRightsDecisionsDecisionIdParams,
  options?: RequestOptions
) {
  const { id: param0, decision_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpRightsDecisionResponseDTO>(
    `/api/v1/source-endpoints/${param0}/rights-decisions/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Evaluate exact current source rights actions POST /api/v1/source-endpoints/${param0}/rights-evaluations */
export async function postSourceEndpointsIdRightsEvaluations(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postSourceEndpointsIdRightsEvaluationsParams,
  body: HotKeyAPI.EvaluateRightsActionsRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpRightsActionMatrixResponseDTO>(
    `/api/v1/source-endpoints/${param0}/rights-evaluations`,
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

/** List source endpoint rights policies GET /api/v1/source-endpoints/${param0}/rights-policies */
export async function getSourceEndpointsIdRightsPolicies(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getSourceEndpointsIdRightsPoliciesParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpRightsPolicyPageResponseDTO>(
    `/api/v1/source-endpoints/${param0}/rights-policies`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Create an immutable source rights policy POST /api/v1/source-endpoints/${param0}/rights-policies */
export async function postSourceEndpointsIdRightsPolicies(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postSourceEndpointsIdRightsPoliciesParams,
  body: HotKeyAPI.CreateRightsPolicyRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpCreateRightsPolicyResponseDTO>(
    `/api/v1/source-endpoints/${param0}/rights-policies`,
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
