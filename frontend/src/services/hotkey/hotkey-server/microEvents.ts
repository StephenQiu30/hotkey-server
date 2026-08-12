// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List semantic micro-events GET /api/v1/micro-events */
export async function getMicroEvents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMicroEventsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.MicroEventV2ResultHttpMicroEventPageResponseDTO>(
    "/api/v1/micro-events",
    {
      method: "GET",
      params: {
        // sort has a default value: heat
        sort: "heat",

        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Get semantic micro-event GET /api/v1/micro-events/${param0} */
export async function getMicroEventsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMicroEventsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MicroEventV2ResultHttpMicroEventResponseDTO>(
    `/api/v1/micro-events/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** List micro-event evidence GET /api/v1/micro-events/${param0}/evidence */
export async function getMicroEventsIdEvidence(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMicroEventsIdEvidenceParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MicroEventV2ResultHttpMicroEventEvidencePageResponseDTO>(
    `/api/v1/micro-events/${param0}/evidence`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Append manually reviewed claim evidence POST /api/v1/micro-events/${param0}/evidence */
export async function postMicroEventsIdEvidence(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMicroEventsIdEvidenceParams,
  body: HotKeyAPI.RecordClaimEvidenceRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MicroEventV2ResultHttpClaimEvidenceMutationResponseDTO>(
    `/api/v1/micro-events/${param0}/evidence`,
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

/** Correct a claim evidence relation or locator POST /api/v1/micro-events/${param0}/evidence/${param1}/feedback */
export async function postMicroEventsIdEvidenceEvidenceIdFeedback(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMicroEventsIdEvidenceEvidenceIdFeedbackParams,
  body: HotKeyAPI.CorrectClaimEvidenceRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, evidence_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.MicroEventV2ResultHttpClaimEvidenceCorrectionResponseDTO>(
    `/api/v1/micro-events/${param0}/evidence/${param1}/feedback`,
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

/** Apply micro-event governance feedback POST /api/v1/micro-events/${param0}/feedback */
export async function postMicroEventsIdFeedback(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMicroEventsIdFeedbackParams,
  body: HotKeyAPI.MicroEventGovernanceRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MicroEventV2ResultHttpMicroEventGovernanceResponseDTO>(
    `/api/v1/micro-events/${param0}/feedback`,
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
