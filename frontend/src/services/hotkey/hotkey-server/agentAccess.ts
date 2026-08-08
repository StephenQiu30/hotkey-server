// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** List current user's Agent Tokens GET /api/v1/agent-tokens */
export async function getAgentTokens(options?: RequestOptions) {
  return request<HotKeyAPI.AgentAccessResultArrayHttpTokenResponse>(
    "/api/v1/agent-tokens",
    {
      method: "GET",
      ...(options || {}),
    }
  );
}

/** Create an Agent Token POST /api/v1/agent-tokens */
export async function postAgentTokens(
  body: HotKeyAPI.CreateTokenRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.AgentAccessResultHttpCreatedTokenResponse>(
    "/api/v1/agent-tokens",
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

/** Revoke an Agent Token POST /api/v1/agent-tokens/${param0}/revoke */
export async function postAgentTokensIdRevoke(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postAgentTokensIdRevokeParams,
  body: HotKeyAPI.RevokeTokenRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.AgentAccessResultHttpTokenResponse>(
    `/api/v1/agent-tokens/${param0}/revoke`,
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
