import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getOperationsUsage: vi.fn(),
  getOperationsRetentionPolicies: vi.fn(),
  getOperationsRetentionRunsId: vi.fn(),
  getOperationsAuditLogs: vi.fn(),
  getOperationsOverview: vi.fn(),
  getOperationsJobs: vi.fn(),
  postOperationsJobsIdCancel: vi.fn(),
  postOperationsJobsIdRetry: vi.fn(),
  postOperationsRetentionPoliciesIdPreview: vi.fn(),
  postOperationsRetentionRunsIdApprove: vi.fn(),
  postOperationsRetentionRunsIdExecute: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/operations", () => mocks);

import GovernancePage from "@/app/dashboard/governance/page";

const usage = [
  { dimension: "active_monitors", label: "活跃监控", mode: "hard", used: "12", limit: "50", remaining: "38", unit: "个" },
  { dimension: "manual_searches", label: "手动搜索", mode: "observed", used: "3", unit: "次" },
  { dimension: "source_calls", label: "来源调用", mode: "observed", used: "28", unit: "次" },
  { dimension: "ai_tokens", label: "AI Token", mode: "observed", used: "1200", unit: "tokens" },
  { dimension: "ai_cost", label: "AI 成本", mode: "hard", used: "1.6", limit: "10", reserved: "0.4", settled: "1.2", unit: "USD" },
  { dimension: "notification_deliveries", label: "通知投递", mode: "observed", used: "5", unit: "次" },
] satisfies HotKeyAPI.UsageItem[];

const policies = [
  { id: 1, version: 1, data_class: "content_metric_snapshots", retention_days: 180, action: "delete", enabled: true, protected: false, description: "删除过期内容指标快照" },
  { id: 2, version: 1, data_class: "audit_logs", retention_days: 365, action: "delete", enabled: false, protected: true, description: "受保护的审计事实" },
  { id: 3, version: 1, data_class: "delivery_attempts", retention_days: 90, action: "delete", enabled: false, protected: true, description: "受保护的投递尝试事实" },
] satisfies HotKeyAPI.RetentionPolicyResponse[];

function setRole(role: UserRole) {
  useAuthStore.setState({ status: AuthStatus.Authenticated, user: { id: 1, email: "actor@example.test", role }, error: null });
}

describe("GovernancePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setRole(UserRole.Admin);
    mocks.getOperationsUsage.mockResolvedValue({ data: { items: usage } });
    mocks.getOperationsRetentionPolicies.mockResolvedValue({ data: policies });
    mocks.getOperationsAuditLogs.mockResolvedValue({ data: { items: [{ id: 10, action: "monitor.published", resource_type: "monitor", resource_id: 7, actor_type: "user", actor_id: 1, result: "success", created_at: "2026-08-08T08:00:00Z" }] } });
    mocks.getOperationsOverview.mockResolvedValue({ data: {
      alert_policy_version: "p0-operational-alerts-v1",
      available_jobs: 2,
      running_jobs: 1,
      completed_jobs: 8,
      discarded_jobs: 1,
      cancelled_jobs: 0,
      queue_lag_seconds: 45,
      alerts: [
        {
          alert_id: "ALERT-RIVER-JOB-FAILED",
          policy_version: "p0-operational-alerts-v1",
          severity: "p1",
          owner: "hotkey-oncall",
          silence_key: "ALERT-RIVER-JOB-FAILED",
          threshold_count: 1,
          reason_code: "river_job_discarded",
          runbook_url: "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-observability.md#river-alert-response",
          job_id: 31,
          event_id: 42,
          trace_id: "0123456789abcdef0123456789abcdef",
          affected_count: 1,
          triggered_at: "2026-08-08T08:00:00Z",
        },
        {
          alert_id: "ALERT-DELIVERY-UNKNOWN",
          policy_version: "p0-operational-alerts-v1",
          severity: "p1",
          owner: "hotkey-oncall",
          silence_key: "ALERT-DELIVERY-UNKNOWN",
          threshold_count: 1,
          reason_code: "notification_delivery_unknown",
          runbook_url: "https://github.com/StephenQiu30/hotkey-server/blob/main/docs/operations/004-observability.md#delivery-unknown-alert-response",
          attempt_id: 91,
          notification_id: 52,
          resource_type: "micro_event",
          resource_id: 43,
          affected_count: 2,
          triggered_at: "2026-08-08T08:01:00Z",
        },
      ],
    } });
    mocks.getOperationsJobs.mockResolvedValue({ data: { items: [
      { id: 31, kind: "collect_source", state: "discarded", attempt: 3, max_attempts: 3, priority: 1, resource_id: 7, failure_code: "retryable", scheduled_at: "2026-08-08T08:00:00Z", created_at: "2026-08-08T08:00:00Z" },
      { id: 32, kind: "build_report", state: "available", attempt: 0, max_attempts: 3, priority: 1, resource_id: 9, scheduled_at: "2026-08-08T08:01:00Z", created_at: "2026-08-08T08:01:00Z" },
    ] } });
    mocks.postOperationsJobsIdCancel.mockResolvedValue({ data: { id: 32, state: "cancelled" } });
    mocks.postOperationsJobsIdRetry.mockResolvedValue({ data: { id: 31, state: "available" } });
    mocks.getOperationsRetentionRunsId.mockResolvedValue({ data: { run_id: 11, policy_version: 1, candidate_hash: "a".repeat(64), status: "pending_approval", data_class: "content_metric_snapshots", affected: 2, batch_size: 100, cutoff: "2026-02-09T00:00:00Z", dry_run: true, has_more: true, requested_by_user_id: 2 } });
    mocks.postOperationsRetentionPoliciesIdPreview.mockResolvedValue({ data: { run_id: 11, policy_version: 1, candidate_hash: "a".repeat(64), status: "pending_approval", data_class: "content_metric_snapshots", affected: 2, batch_size: 100, cutoff: "2026-02-09T00:00:00Z", dry_run: true, has_more: true, requested_by_user_id: 1 } });
    mocks.postOperationsRetentionRunsIdApprove.mockResolvedValue({ data: { run_id: 11, policy_version: 1, candidate_hash: "a".repeat(64), status: "approved", data_class: "content_metric_snapshots", affected: 2, batch_size: 100, cutoff: "2026-02-09T00:00:00Z", dry_run: true, has_more: true, requested_by_user_id: 2, approved_by_user_id: 1 } });
    mocks.postOperationsRetentionRunsIdExecute.mockResolvedValue({ data: { run_id: 11, policy_version: 1, candidate_hash: "a".repeat(64), status: "completed", data_class: "content_metric_snapshots", affected: 2, batch_size: 100, cutoff: "2026-02-09T00:00:00Z", dry_run: false, has_more: false, requested_by_user_id: 2, approved_by_user_id: 1 } });
  });

  it("does not render or load governance for non-admins", () => {
    setRole(UserRole.Editor);
    const { container } = render(<GovernancePage />);

    expect(container).toBeEmptyDOMElement();
    expect(mocks.getOperationsUsage).not.toHaveBeenCalled();
  });

  it("renders five fact-backed usage groups and safe audit facts", async () => {
    render(<GovernancePage />);
    expect(await screen.findByText(/剩余 38 \/ 50/)).toBeInTheDocument();
    expect(screen.getByText(/1200 tokens · \$1.2/)).toBeInTheDocument();
    expect(screen.getByRole("progressbar", { name: "AI Token / 成本 已使用 1200 tokens · $1.2，上限 10" })).toHaveAttribute("aria-valuenow", "16");
    expect(screen.getByText("monitor.published")).toBeInTheDocument();
    expect(screen.getByText("审计日志")).toBeInTheDocument();
    expect(screen.getByText("投递尝试（受保护）")).toBeInTheDocument();
    expect(screen.getAllByText("受保护")).toHaveLength(2);
    expect(screen.getByRole("heading", { name: "运行状态" })).toBeInTheDocument();
    expect(screen.getByText("collect_source")).toBeInTheDocument();
    expect(screen.getByText("retryable")).toBeInTheDocument();
    expect(screen.getByText("45 秒")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "运行告警" })).toBeInTheDocument();
    expect(screen.getByText("策略 p0-operational-alerts-v1")).toBeInTheDocument();
    expect(screen.getAllByText("责任人 hotkey-oncall")).toHaveLength(2);
    expect(screen.getByText("ALERT-RIVER-JOB-FAILED")).toBeInTheDocument();
    expect(screen.getByText(/任务 #31/)).toBeInTheDocument();
    expect(screen.getByText(/事件 #42/)).toBeInTheDocument();
    expect(screen.getByText("0123456789abcdef0123456789abcdef")).toBeInTheDocument();
    expect(screen.getByText("ALERT-DELIVERY-UNKNOWN")).toBeInTheDocument();
    expect(screen.getByText("影响 2 个交付")).toBeInTheDocument();
    expect(screen.getByText(/通知 #52/)).toBeInTheDocument();
    expect(screen.getByText(/尝试 #91/)).toBeInTheDocument();
    expect(screen.getByText(/micro_event #43/)).toBeInTheDocument();
    const runbookLinks = screen.getAllByRole("link", { name: "打开处置手册" });
    expect(runbookLinks[0]).toHaveAttribute("href", expect.stringContaining("#river-alert-response"));
    expect(runbookLinks[1]).toHaveAttribute("href", expect.stringContaining("#delivery-unknown-alert-response"));
  });

  it("labels every durable-fact alert with its bounded impact and threshold", async () => {
    const alerts = [
      ["ALERT-SOURCE-AUTH", "来源", 3, 0],
      ["ALERT-MINIO-WRITE", "证据异常", 1, 0],
      ["ALERT-CODEX-FAILURE", "智能任务", 3, 0],
      ["ALERT-VAULT-CONFLICT", "冲突", 1, 0],
      ["ALERT-BACKUP-FAILED", "备份运行", 1, 900],
      ["ALERT-SEARCH-BACKLOG", "检索任务", 1, 300],
    ] as const;
    mocks.getOperationsOverview.mockResolvedValue({ data: {
      alert_policy_version: "p0-operational-alerts-v1",
      alerts: alerts.map(([alertID, , thresholdCount, thresholdSeconds], index) => ({
        alert_id: alertID,
        policy_version: "p0-operational-alerts-v1",
        severity: "p1",
        owner: "hotkey-oncall",
        reason_code: `fixture_${index}`,
        resource_type: "fixture",
        resource_id: index + 1,
        affected_count: index + 1,
        threshold_count: thresholdCount,
        threshold_seconds: thresholdSeconds,
        runbook_url: `https://example.test/runbook#${alertID}`,
      })),
    } });

    render(<GovernancePage />);

    for (const [alertID, unit] of alerts) {
      expect(await screen.findByText(alertID)).toBeInTheDocument();
      expect(screen.getByText(new RegExp(`个${unit}$`))).toBeInTheDocument();
    }
    expect(screen.getAllByText(/阈值 3 次$/)).toHaveLength(2);
    expect(screen.getByText(/阈值 900 秒$/)).toBeInTheDocument();
    expect(screen.getByText(/阈值 300 秒$/)).toBeInTheDocument();
  });

  it("filters jobs and confirms bounded retry or cancellation", async () => {
    render(<GovernancePage />);
    const user = userEvent.setup();
    await screen.findByText("collect_source");
    await user.click(screen.getByRole("combobox", { name: "筛选任务状态" }));
    await user.click(screen.getByRole("option", { name: "失败" }));
    await waitFor(() => expect(mocks.getOperationsJobs).toHaveBeenLastCalledWith({ limit: 20, state: "discarded" }));
    const retryButton = screen.getByRole("button", { name: "重试任务 31" });
    await user.click(retryButton);
    expect(screen.getByRole("alertdialog", { name: "确认重试任务？" })).toHaveTextContent("不会修改原始任务参数");
    await user.keyboard("{Escape}");
    expect(retryButton).toHaveFocus();
    await user.click(retryButton);
    await user.click(screen.getByRole("button", { name: "确认重试" }));
    await waitFor(() => expect(mocks.postOperationsJobsIdRetry).toHaveBeenCalledWith({ id: 31 }));
    await waitFor(() => expect(screen.getByRole("button", { name: "刷新运行状态" })).toHaveFocus());
  });

  it("renders an explicit empty state for a job filter without results", async () => {
    mocks.getOperationsJobs.mockResolvedValue({ data: { items: [] } });
    render(<GovernancePage />);
    expect(await screen.findByText("暂无匹配任务")).toBeInTheDocument();
    expect(screen.getByText("调整状态筛选，或等待后台调度创建任务。")).toBeInTheDocument();
  });

  it("recovers the runtime panel after a bounded retry", async () => {
    mocks.getOperationsOverview.mockRejectedValueOnce(new Error("runtime unavailable"));
    render(<GovernancePage />);
    const user = userEvent.setup();
    expect(await screen.findByText("无法加载运行状态")).toBeInTheDocument();
    expect(screen.getByText("runtime unavailable")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "重试运行状态" }));
    expect(await screen.findByText("45 秒")).toBeInTheDocument();
  });

  it("requires a second administrator to approve an initiator's frozen retention run", async () => {
    render(<GovernancePage />);
    const user = userEvent.setup();
    const previewButtons = await screen.findAllByRole("button", { name: "预览清理" });
    await user.click(previewButtons.find((button) => !button.hasAttribute("disabled"))!);
    await waitFor(() => expect(mocks.postOperationsRetentionPoliciesIdPreview).toHaveBeenCalledWith({ id: 1 }, { expected_version: 1, batch_size: 100 }));
    const dialog = screen.getByRole("alertdialog", { name: "等待另一名管理员批准" });
    expect(dialog).toHaveTextContent("运行 #11");
    expect(dialog).toHaveTextContent("a".repeat(64));
    expect(dialog).toHaveTextContent("发起人与批准人必须不同");
    expect(screen.queryByRole("button", { name: "批准固定清单" })).not.toBeInTheDocument();
    expect(mocks.postOperationsRetentionRunsIdApprove).not.toHaveBeenCalled();
    expect(mocks.postOperationsRetentionRunsIdExecute).not.toHaveBeenCalled();
  });

  it("loads a handed-off retention run for independent approval and execution", async () => {
    render(<GovernancePage />);
    const user = userEvent.setup();
    await screen.findByText("monitor.published");
    await user.type(screen.getByLabelText("待审批保留运行 ID"), "11");
    await user.click(screen.getByRole("button", { name: "加载待审批运行" }));
    await waitFor(() => expect(mocks.getOperationsRetentionRunsId).toHaveBeenCalledWith({ id: 11 }));
    expect(screen.getByRole("alertdialog", { name: "批准固定保留清单？" })).toHaveTextContent("发起人与批准人必须不同");
    await user.click(screen.getByRole("button", { name: "批准固定清单" }));
    await waitFor(() => expect(mocks.postOperationsRetentionRunsIdApprove).toHaveBeenCalledWith({ id: 11 }, { candidate_hash: "a".repeat(64) }));
    expect(screen.getByRole("alertdialog", { name: "执行已批准保留批次？" })).toHaveTextContent("候选 Hash 已冻结");
    await user.click(screen.getByRole("button", { name: "执行 2 条" }));
    await waitFor(() => expect(mocks.postOperationsRetentionRunsIdExecute).toHaveBeenCalledWith({ id: 11 }, { candidate_hash: "a".repeat(64) }));
  });

  it("applies audit filters through the generated query client", async () => {
    render(<GovernancePage />);
    const user = userEvent.setup();
    await screen.findByText("monitor.published");
    await user.click(screen.getByRole("combobox", { name: "筛选审计动作" }));
    await user.click(screen.getByRole("option", { name: "保留执行" }));
    await user.click(screen.getByRole("button", { name: "应用筛选" }));
    await waitFor(() => expect(mocks.getOperationsAuditLogs).toHaveBeenLastCalledWith({ limit: 20, action: "retention.executed" }));
  });

  it("offers a retry when initial governance loading fails", async () => {
    mocks.getOperationsUsage.mockRejectedValueOnce(new Error("governance unavailable"));
    render(<GovernancePage />);
    const user = userEvent.setup();
    expect(await screen.findByText("governance unavailable")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "重试" }));
    expect(await screen.findByText(/剩余 38 \/ 50/)).toBeInTheDocument();
  });
});
