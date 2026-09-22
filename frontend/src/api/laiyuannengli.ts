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

/** 配置或启停来源连接 仅使用服务端已配置凭据; 版本变化后必须重新验证, 历史资料保留。 PUT /api/source-connections/${param0} */
export async function updateSourceConnection(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.updateSourceConnectionParams,
  body: HotKeyAPI.SourceConnectionUpdateInput,
  options?: import("@/request").RequestOptions,
) {
  const { source_key: param0, ...queryParams } = params;
  return request<HotKeyAPI.SourceConnectionView>(
    `/api/source-connections/${param0}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}
