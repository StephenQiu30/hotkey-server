// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "../request";

/** Me GET /api/session */
export async function getSession(options?: RequestOptions) {
  return request<API.Principal>("/api/session", {
    method: "GET",
    ...(options || {}),
  });
}

/** Login POST /api/session */
export async function login(body: API.LoginInput, options?: RequestOptions) {
  return request<API.Principal>("/api/session", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Logout DELETE /api/session */
export async function logout(options?: RequestOptions) {
  return request<any>("/api/session", {
    method: "DELETE",
    ...(options || {}),
  });
}
