// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Inbox Contents GET /api/v1/contents */
export async function listInboxContents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listInboxContentsParams,
  options?: RequestOptions
) {
  return request<API.InboxPage>("/api/v1/contents", {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",
      ...params,
    },
    ...(options || {}),
  });
}
