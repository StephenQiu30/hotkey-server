// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List audit logs GET /api/v1/operations/audit-logs */
export async function getOperationsAuditLogs(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getOperationsAuditLogsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.GovernanceResultDomainAuditPage>(
    "/api/v1/operations/audit-logs",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** List durable jobs GET /api/v1/operations/jobs */
export async function getOperationsJobs(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getOperationsJobsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.JobResultHttpJobPageResponse>(
    "/api/v1/operations/jobs",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Cancel a durable job POST /api/v1/operations/jobs/${param0}/cancel */
export async function postOperationsJobsIdCancel(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postOperationsJobsIdCancelParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.JobResultHttpJobResponse>(
    `/api/v1/operations/jobs/${param0}/cancel`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Retry a durable job POST /api/v1/operations/jobs/${param0}/retry */
export async function postOperationsJobsIdRetry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postOperationsJobsIdRetryParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.JobResultHttpJobResponse>(
    `/api/v1/operations/jobs/${param0}/retry`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Get runtime overview GET /api/v1/operations/overview */
export async function getOperationsOverview(options?: RequestOptions) {
  return request<HotKeyAPI.OverviewResultDomainRuntimeOverview>(
    "/api/v1/operations/overview",
    {
      method: "GET",
      ...(options || {}),
    }
  );
}

/** List retention policies GET /api/v1/operations/retention-policies */
export async function getOperationsRetentionPolicies(options?: RequestOptions) {
  return request<HotKeyAPI.GovernanceResultArrayHttpRetentionPolicyResponse>(
    "/api/v1/operations/retention-policies",
    {
      method: "GET",
      ...(options || {}),
    }
  );
}

/** Preview a retention batch POST /api/v1/operations/retention-policies/${param0}/preview */
export async function postOperationsRetentionPoliciesIdPreview(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postOperationsRetentionPoliciesIdPreviewParams,
  body: HotKeyAPI.RetentionRunRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.GovernanceResultDomainCleanupResult>(
    `/api/v1/operations/retention-policies/${param0}/preview`,
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

/** Run a retention batch POST /api/v1/operations/retention-policies/${param0}/run */
export async function postOperationsRetentionPoliciesIdRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postOperationsRetentionPoliciesIdRunParams,
  body: HotKeyAPI.RetentionRunRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.GovernanceResultDomainCleanupResult>(
    `/api/v1/operations/retention-policies/${param0}/run`,
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

/** Get quota and usage overview GET /api/v1/operations/usage */
export async function getOperationsUsage(options?: RequestOptions) {
  return request<HotKeyAPI.GovernanceResultDomainUsageOverview>(
    "/api/v1/operations/usage",
    {
      method: "GET",
      ...(options || {}),
    }
  );
}
