// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Create a metric capability profile draft POST /api/v1/metric-capability-profiles */
export async function postMetricCapabilityProfiles(
  body: HotKeyAPI.CreateMetricCapabilityProfileRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.SourceResultHttpMetricCapabilityProfileResponse>(
    "/api/v1/metric-capability-profiles",
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

/** Archive a metric capability profile POST /api/v1/metric-capability-profiles/${param0}/archive */
export async function postMetricCapabilityProfilesIdArchive(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMetricCapabilityProfilesIdArchiveParams,
  body: HotKeyAPI.MetricCapabilityLifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpMetricCapabilityProfileResponse>(
    `/api/v1/metric-capability-profiles/${param0}/archive`,
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

/** Publish a metric capability profile POST /api/v1/metric-capability-profiles/${param0}/publish */
export async function postMetricCapabilityProfilesIdPublish(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMetricCapabilityProfilesIdPublishParams,
  body: HotKeyAPI.MetricCapabilityLifecycleRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceResultHttpMetricCapabilityProfileResponse>(
    `/api/v1/metric-capability-profiles/${param0}/publish`,
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
