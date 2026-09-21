// @ts-ignore
/* eslint-disable */
import request from "@/request";

/** 初始化首个使用者 需要部署者配置的 bootstrap 凭据。成功后建立唯一 owner 与首个会话。 POST /api/identity/initialize */
export async function initializeOwner(
  body: HotKeyAPI.IdentityCredentialsInput,
  options?: import("@/request").RequestOptions,
) {
  return request<HotKeyAPI.IdentitySessionView>("/api/identity/initialize", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 读取当前会话 需要有效的 SessionCookie。只返回非敏感的使用者和过期时间。 GET /api/identity/session */
export async function getIdentitySession(
  options?: import("@/request").RequestOptions,
) {
  return request<HotKeyAPI.IdentitySessionView>("/api/identity/session", {
    method: "GET",
    ...(options || {}),
  });
}

/** 注销当前会话 需要有效的 SessionCookie 与匹配的 X-HotKey-CSRF 请求头。 DELETE /api/identity/session */
export async function deleteIdentitySession(
  options?: import("@/request").RequestOptions,
) {
  return request<any>("/api/identity/session", {
    method: "DELETE",
    ...(options || {}),
  });
}

/** 登录并建立会话 验证 owner 凭据并设置服务端可撤销的不透明会话 Cookie。 POST /api/identity/sessions */
export async function createIdentitySession(
  body: HotKeyAPI.IdentityCredentialsInput,
  options?: import("@/request").RequestOptions,
) {
  return request<HotKeyAPI.IdentitySessionView>("/api/identity/sessions", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
