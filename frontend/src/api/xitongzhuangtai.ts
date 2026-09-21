// @ts-ignore
/* eslint-disable */
import request from "@/request";

/** 查询服务存活状态 GET /api/health */
export async function getHealth(options?: import("@/request").RequestOptions) {
  return request<HotKeyAPI.HealthView>("/api/health", {
    method: "GET",
    ...(options || {}),
  });
}

/** 查询服务就绪状态 GET /api/ready */
export async function getReadiness(
  options?: import("@/request").RequestOptions,
) {
  return request<HotKeyAPI.HealthView>("/api/ready", {
    method: "GET",
    ...(options || {}),
  });
}
