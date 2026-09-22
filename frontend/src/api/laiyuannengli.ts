// @ts-ignore
/* eslint-disable */
import request from "@/request";

/** 列出来源能力 按当前 owner 返回平台目录、连接版本及手动/定时入口的持久状态。 GET /api/source-capabilities */
export async function listSourceCapabilities(
  options?: import("@/request").RequestOptions,
) {
  return request<HotKeyAPI.PageViewSourcePlatformView_>(
    "/api/source-capabilities",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}
