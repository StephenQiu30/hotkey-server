// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Review a content lineage decision POST /api/v1/content-lineage-decisions/${param0}/feedback */
export async function postContentLineageDecisionsIdFeedback(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postContentLineageDecisionsIdFeedbackParams,
  body: HotKeyAPI.ReviewContentLineageRequestDTO,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.ContentResultHttpContentLineageFeedbackResponseDTO>(
    `/api/v1/content-lineage-decisions/${param0}/feedback`,
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
