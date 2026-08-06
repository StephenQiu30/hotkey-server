// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Record unmatched-content false-negative feedback PUT /api/v1/monitors/${param0}/contents/${param1}/feedback */
export async function putMonitorsIdContentsContentIdFeedback(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.putMonitorsIdContentsContentIdFeedbackParams,
  body: HotKeyAPI.RelevanceFalseNegativeFeedbackRequest,
  options?: RequestOptions
) {
  const { id: param0, content_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpRelevanceFeedbackResponse>(
    `/api/v1/monitors/${param0}/contents/${param1}/feedback`,
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

/** Get relevance feedback evaluation GET /api/v1/monitors/${param0}/feedback/evaluation */
export async function getMonitorsIdFeedbackEvaluation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMonitorsIdFeedbackEvaluationParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultArrayHttpRelevanceEvaluationResponse>(
    `/api/v1/monitors/${param0}/feedback/evaluation`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** List relevance feedback suggestions GET /api/v1/monitors/${param0}/feedback/suggestions */
export async function getMonitorsIdFeedbackSuggestions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMonitorsIdFeedbackSuggestionsParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpRelevanceSuggestionPageResponse>(
    `/api/v1/monitors/${param0}/feedback/suggestions`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Review a relevance feedback suggestion POST /api/v1/monitors/${param0}/feedback/suggestions/${param1}/review */
export async function postMonitorsIdFeedbackSuggestionsSuggestionIdReview(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdFeedbackSuggestionsSuggestionIdReviewParams,
  body: HotKeyAPI.RelevanceSuggestionReviewRequest,
  options?: RequestOptions
) {
  const { id: param0, suggestion_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpRelevanceSuggestionResponse>(
    `/api/v1/monitors/${param0}/feedback/suggestions/${param1}/review`,
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

/** Refresh feedback suggestions POST /api/v1/monitors/${param0}/feedback/suggestions/refresh */
export async function postMonitorsIdFeedbackSuggestionsRefresh(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdFeedbackSuggestionsRefreshParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpRelevanceRefreshResponse>(
    `/api/v1/monitors/${param0}/feedback/suggestions/refresh`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** List relevance matches GET /api/v1/monitors/${param0}/matches */
export async function getMonitorsIdMatches(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMonitorsIdMatchesParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpRelevanceMatchPageResponse>(
    `/api/v1/monitors/${param0}/matches`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    }
  );
}

/** Get a relevance explanation GET /api/v1/monitors/${param0}/matches/${param1} */
export async function getMonitorsIdMatchesMatchId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMonitorsIdMatchesMatchIdParams,
  options?: RequestOptions
) {
  const { id: param0, match_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpRelevanceMatchDetailResponse>(
    `/api/v1/monitors/${param0}/matches/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Upsert feedback for a relevance match PUT /api/v1/monitors/${param0}/matches/${param1}/feedback */
export async function putMonitorsIdMatchesMatchIdFeedback(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.putMonitorsIdMatchesMatchIdFeedbackParams,
  body: HotKeyAPI.RelevanceFeedbackRequest,
  options?: RequestOptions
) {
  const { id: param0, match_id: param1, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpRelevanceFeedbackResponse>(
    `/api/v1/monitors/${param0}/matches/${param1}/feedback`,
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

/** Preview relevance scoring without writes POST /api/v1/monitors/${param0}/relevance-preview */
export async function postMonitorsIdRelevancePreview(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postMonitorsIdRelevancePreviewParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultArrayHttpRelevancePreviewItemResponse>(
    `/api/v1/monitors/${param0}/relevance-preview`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
