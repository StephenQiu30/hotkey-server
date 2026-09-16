// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Inbox Contents GET /api/contents */
export async function listInboxContents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listInboxContentsParams,
  options?: RequestOptions
) {
  return request<API.InboxPage>("/api/contents", {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",

      ...params,
    },
    ...(options || {}),
  });
}

/** Withdraw Content POST /api/contents/${param0}/withdraw */
export async function withdrawContent(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.withdrawContentParams,
  body: API.ContentWithdrawalInput,
  options?: RequestOptions
) {
  const { identity: param0, ...queryParams } = params;
  return request<API.ContentWithdrawalView>(
    `/api/contents/${param0}/withdraw`,
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
