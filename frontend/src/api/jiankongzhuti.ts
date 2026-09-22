// @ts-ignore
/* eslint-disable */
import request from "@/request";

/** 列出监控主题 按当前 owner 列出主题; 默认隐藏已归档主题。 GET /api/topics */
export async function listMonitorTopics(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.listMonitorTopicsParams,
  options?: import("@/request").RequestOptions,
) {
  return request<HotKeyAPI.PageViewMonitorTopicView_>("/api/topics", {
    method: "GET",
    params: {
      // limit has a default value: 20
      limit: "20",
      ...params,
    },
    ...(options || {}),
  });
}

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

/** 归档监控主题 归档主题并保留规则历史; 不删除资料; 不假报在途任务已取消。 POST /api/topics/${param0}/archive */
export async function archiveMonitorTopic(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.archiveMonitorTopicParams,
  options?: import("@/request").RequestOptions,
) {
  const { topic_id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorTopicView>(`/api/topics/${param0}/archive`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 复制监控主题 复制当前规则为新主题版本 1; 新主题固定暂停且不复制旧任务。 POST /api/topics/${param0}/clone */
export async function cloneMonitorTopic(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.cloneMonitorTopicParams,
  options?: import("@/request").RequestOptions,
) {
  const { topic_id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorTopicView>(`/api/topics/${param0}/clone`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 暂停监控主题 暂停后不允许后续调度; 已运行任务仍需在任务详情单独取消。 POST /api/topics/${param0}/pause */
export async function pauseMonitorTopic(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.pauseMonitorTopicParams,
  options?: import("@/request").RequestOptions,
) {
  const { topic_id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorTopicView>(`/api/topics/${param0}/pause`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 恢复监控主题 仅来源已就绪的暂停主题可恢复; 保存主题本身不启动采集。 POST /api/topics/${param0}/resume */
export async function resumeMonitorTopic(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.resumeMonitorTopicParams,
  options?: import("@/request").RequestOptions,
) {
  const { topic_id: param0, ...queryParams } = params;
  return request<HotKeyAPI.MonitorTopicView>(`/api/topics/${param0}/resume`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
