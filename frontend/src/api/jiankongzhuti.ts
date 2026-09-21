// @ts-ignore
/* eslint-disable */
import request from "@/request";

/** 创建监控主题 保存本地匹配规则版本; 来源未选定时主题保持暂停。 POST /api/topics */
export async function createMonitorTopic(
  body: HotKeyAPI.MonitorTopicCreateInput,
  options?: import("@/request").RequestOptions,
) {
  return request<HotKeyAPI.MonitorTopicView>("/api/topics", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 读取监控主题 按当前会话 owner 读取主题和当前不可变规则版本。 GET /api/topics/${param0} */
export async function getMonitorTopic(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getMonitorTopicParams,
  options?: import("@/request").RequestOptions,
) {
  const { topic_id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorTopicView>(`/api/topics/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 编辑监控主题 使用 expected_version 防止覆盖并发修改; 规则变化创建新版本。 PATCH /api/topics/${param0} */
export async function updateMonitorTopic(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.updateMonitorTopicParams,
  body: HotKeyAPI.MonitorTopicUpdateInput,
  options?: import("@/request").RequestOptions,
) {
  const { topic_id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorTopicView>(`/api/topics/${param0}`, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
