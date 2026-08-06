// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List AI model profiles GET /api/v1/ai/model-profiles */
export async function getAiModelProfiles(options?: RequestOptions) {
  return request<HotKeyAPI.ModelProfileResultHttpModelProfileListResponse>(
    "/api/v1/ai/model-profiles",
    {
      method: "GET",
      ...(options || {}),
    }
  );
}

/** Create an AI model profile POST /api/v1/ai/model-profiles */
export async function postAiModelProfiles(
  body: HotKeyAPI.CreateModelProfileRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.ModelProfileResultHttpModelProfileResponse>(
    "/api/v1/ai/model-profiles",
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

/** Get an AI model profile GET /api/v1/ai/model-profiles/${param0} */
export async function getAiModelProfilesId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getAiModelProfilesIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ModelProfileResultHttpModelProfileResponse>(
    `/api/v1/ai/model-profiles/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Archive an AI model profile DELETE /api/v1/ai/model-profiles/${param0} */
export async function deleteAiModelProfilesId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.deleteAiModelProfilesIdParams,
  body: HotKeyAPI.ModelProfileVersionRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ModelProfileResultHttpModelProfileResponse>(
    `/api/v1/ai/model-profiles/${param0}`,
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

/** Update AI model profile operational settings PATCH /api/v1/ai/model-profiles/${param0} */
export async function patchAiModelProfilesId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.patchAiModelProfilesIdParams,
  body: HotKeyAPI.UpdateModelProfileRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ModelProfileResultHttpModelProfileResponse>(
    `/api/v1/ai/model-profiles/${param0}`,
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

/** Restore an AI model profile POST /api/v1/ai/model-profiles/${param0}/restore */
export async function postAiModelProfilesIdRestore(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postAiModelProfilesIdRestoreParams,
  body: HotKeyAPI.ModelProfileVersionRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ModelProfileResultHttpModelProfileResponse>(
    `/api/v1/ai/model-profiles/${param0}/restore`,
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
