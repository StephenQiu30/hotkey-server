// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Me GET /api/v1/session */
export async function getSession(options?: RequestOptions) {
  return request<API.Principal>("/api/v1/session", {
    method: "GET",
    ...(options || {}),
  });
}

/** Login POST /api/v1/session */
export async function login(body: API.LoginInput, options?: RequestOptions) {
  return request<API.Principal>("/api/v1/session", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Logout DELETE /api/v1/session */
export async function logout(options?: RequestOptions) {
  return request<any>("/api/v1/session", {
    method: "DELETE",
    ...(options || {}),
  });
}
