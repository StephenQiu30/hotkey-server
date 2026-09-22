// @ts-ignore
/* eslint-disable */
import request from "@/request";

/** 提交采集任务 任务与 Outbox 持久提交后才返回受理, 相同操作标识复用原任务。 POST /api/jobs */
export async function createCollectionJob(
  body: HotKeyAPI.CollectionJobInput | HotKeyAPI.WebPageCollectionJobInput,
  options?: import("@/request").RequestOptions,
) {
  return request<HotKeyAPI.JobAcceptedView>("/api/jobs", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 读取采集任务状态 按当前会话 owner 读取持久任务, 不会暴露 scope、租约或内部消息。 GET /api/jobs/${param0} */
export async function getCollectionJob(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.getCollectionJobParams,
  options?: import("@/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<HotKeyAPI.JobStatusView>(`/api/jobs/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 取消采集任务 排队任务立即取消; 运行任务持久化取消意图并等待在途响应收尾。 POST /api/jobs/${param0}/cancel */
export async function cancelCollectionJob(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.cancelCollectionJobParams,
  options?: import("@/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<HotKeyAPI.JobStatusView>(`/api/jobs/${param0}/cancel`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 重试采集任务 恢复同一任务及其检查点; 重复点击已排队任务不会重复派发。 POST /api/jobs/${param0}/retry */
export async function retryCollectionJob(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.retryCollectionJobParams,
  options?: import("@/request").RequestOptions,
) {
  const { job_id: param0, ...queryParams } = params;
  return request<HotKeyAPI.JobStatusView>(`/api/jobs/${param0}/retry`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}
