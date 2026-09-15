// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Publish Analysis Knowledge POST /api/v1/analysis-runs/${param0}/knowledge-entry */
export async function publishAnalysisKnowledge(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.publishAnalysisKnowledgeParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.KnowledgeEntryView>(
    `/api/v1/analysis-runs/${param0}/knowledge-entry`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Search Knowledge GET /api/v1/knowledge */
export async function searchKnowledge(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.searchKnowledgeParams,
  options?: RequestOptions
) {
  return request<API.KnowledgePage>("/api/v1/knowledge", {
    method: "GET",
    params: {
      // mode has a default value: exact_substring
      mode: "exact_substring",
      // limit has a default value: 20
      limit: "20",
      ...params,
    },
    ...(options || {}),
  });
}

/** Get Knowledge Entry GET /api/v1/knowledge/${param0} */
export async function getKnowledgeEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getKnowledgeEntryParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.KnowledgeEntryView>(`/api/v1/knowledge/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** Index Knowledge Entry POST /api/v1/knowledge/${param0}/semantic-index */
export async function indexKnowledgeEntry(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.indexKnowledgeEntryParams,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.KnowledgeEntryView>(
    `/api/v1/knowledge/${param0}/semantic-index`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Query Knowledge POST /api/v1/knowledge/query */
export async function queryKnowledge(
  body: API.CommentCountQuestion | API.EvidenceQuestion,
  options?: RequestOptions
) {
  return request<API.KnowledgeAnswer>("/api/v1/knowledge/query", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
