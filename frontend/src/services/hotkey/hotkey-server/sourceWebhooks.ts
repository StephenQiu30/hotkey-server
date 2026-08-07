// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Receive Bilibili Open Platform webhook POST /api/v1/source-webhooks/bilibili */
export async function postSourceWebhooksBilibili(options?: RequestOptions) {
  return request<HotKeyAPI.SourceResultInternalModulesSourceTransportHttpEmptyResponse>(
    "/api/v1/source-webhooks/bilibili",
    {
      method: "POST",
      ...(options || {}),
    }
  );
}
