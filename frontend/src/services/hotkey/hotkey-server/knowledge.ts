// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Create knowledge proposal POST /api/v1/knowledge/proposals */
export async function postKnowledgeProposals(
  body: HotKeyAPI.ProposalRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.ProposalResultDomainProposal>(
    "/api/v1/knowledge/proposals",
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

/** Apply knowledge proposal POST /api/v1/knowledge/proposals/${param0}/apply */
export async function postKnowledgeProposalsIdApply(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postKnowledgeProposalsIdApplyParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ProposalResultDomainDocument>(
    `/api/v1/knowledge/proposals/${param0}/apply`,
    {
      method: "POST",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Approve knowledge proposal POST /api/v1/knowledge/proposals/${param0}/approve */
export async function postKnowledgeProposalsIdApprove(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postKnowledgeProposalsIdApproveParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ProposalResultDomainProposal>(
    `/api/v1/knowledge/proposals/${param0}/approve`,
    {
      method: "POST",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Reject knowledge proposal POST /api/v1/knowledge/proposals/${param0}/reject */
export async function postKnowledgeProposalsIdReject(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postKnowledgeProposalsIdRejectParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ProposalResultDomainProposal>(
    `/api/v1/knowledge/proposals/${param0}/reject`,
    {
      method: "POST",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Reconcile knowledge Vault POST /api/v1/knowledge/reconcile */
export async function postKnowledgeReconcile(options?: RequestOptions) {
  return request<HotKeyAPI.ProposalResultDomainReconciliationReport>(
    "/api/v1/knowledge/reconcile",
    {
      method: "POST",
      ...(options || {}),
    }
  );
}
