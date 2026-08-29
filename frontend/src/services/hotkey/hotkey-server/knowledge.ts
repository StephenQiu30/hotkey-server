// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List knowledge documents GET /api/v1/knowledge/documents */
export async function getKnowledgeDocuments(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getKnowledgeDocumentsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.ProposalResultHttpDocumentPageResponse>(
    "/api/v1/knowledge/documents",
    {
      method: "GET",
      params: {
        // limit has a default value: 50
        limit: "50",
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Get knowledge document GET /api/v1/knowledge/documents/${param0} */
export async function getKnowledgeDocumentsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getKnowledgeDocumentsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ProposalResultHttpDocumentResponse>(
    `/api/v1/knowledge/documents/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** List knowledge proposals GET /api/v1/knowledge/proposals */
export async function getKnowledgeProposals(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getKnowledgeProposalsParams,
  options?: RequestOptions
) {
  return request<HotKeyAPI.ProposalResultHttpProposalPageResponse>(
    "/api/v1/knowledge/proposals",
    {
      method: "GET",
      params: {
        // limit has a default value: 50
        limit: "50",
        ...params,
      },
      ...(options || {}),
    }
  );
}

/** Create knowledge proposal POST /api/v1/knowledge/proposals */
export async function postKnowledgeProposals(
  body: HotKeyAPI.ProposalRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.ProposalResultHttpProposalResponse>(
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

/** Get knowledge proposal GET /api/v1/knowledge/proposals/${param0} */
export async function getKnowledgeProposalsId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getKnowledgeProposalsIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ProposalResultHttpProposalResponse>(
    `/api/v1/knowledge/proposals/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
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
  return request<HotKeyAPI.ProposalResultHttpDocumentResponse>(
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
  return request<HotKeyAPI.ProposalResultHttpProposalResponse>(
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
  return request<HotKeyAPI.ProposalResultHttpProposalResponse>(
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
  return request<HotKeyAPI.ProposalResultHttpReconciliationResponse>(
    "/api/v1/knowledge/reconcile",
    {
      method: "POST",
      ...(options || {}),
    }
  );
}
