import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getOperationsUsage: vi.fn(),
  getOperationsRetentionPolicies: vi.fn(),
  getOperationsAuditLogs: vi.fn(),
  getOperationsOverview: vi.fn(),
  getOperationsJobs: vi.fn(),
  postOperationsJobsIdCancel: vi.fn(),
  postOperationsJobsIdRetry: vi.fn(),
  postOperationsRetentionPoliciesIdPreview: vi.fn(),
  postOperationsRetentionPoliciesIdRun: vi.fn(),
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
    mocks.getOperationsOverview.mockResolvedValue({ data: { available_jobs: 2, running_jobs: 1, completed_jobs: 8, discarded_jobs: 1, cancelled_jobs: 0, queue_lag_seconds: 45 } });
    mocks.getOperationsJobs.mockResolvedValue({ data: { items: [
      { id: 31, kind: "collect_source", state: "discarded", attempt: 3, max_attempts: 3, priority: 1, resource_id: 7, failure_code: "retryable", scheduled_at: "2026-08-08T08:00:00Z", created_at: "2026-08-08T08:00:00Z" },
      { id: 32, kind: "build_report", state: "available", attempt: 0, max_attempts: 3, priority: 1, resource_id: 9, scheduled_at: "2026-08-08T08:01:00Z", created_at: "2026-08-08T08:01:00Z" },
    ] } });
    mocks.postOperationsJobsIdCancel.mockResolvedValue({ data: { id: 32, state: "cancelled" } });
    mocks.postOperationsJobsIdRetry.mockResolvedValue({ data: { id: 31, state: "available" } });
    mocks.postOperationsRetentionPoliciesIdPreview.mockResolvedValue({ data: { data_class: "content_metric_snapshots", affected: 2, batch_size: 100, cutoff: "2026-02-09T00:00:00Z", dry_run: true, has_more: true } });
    mocks.postOperationsRetentionPoliciesIdRun.mockResolvedValue({ data: { data_class: "content_metric_snapshots", affected: 2, batch_size: 100, cutoff: "2026-02-09T00:00:00Z", dry_run: false, has_more: false } });
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
    expect(screen.getByText("受保护")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "运行状态" })).toBeInTheDocument();
    expect(screen.getByText("collect_source")).toBeInTheDocument();
    expect(screen.getByText("retryable")).toBeInTheDocument();
    expect(screen.getByText("45 秒")).toBeInTheDocument();
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

  it("requires dry-run before a confirmed bounded retention execution", async () => {
    render(<GovernancePage />);
    const user = userEvent.setup();
    const previewButtons = await screen.findAllByRole("button", { name: "预览清理" });
    await user.click(previewButtons.find((button) => !button.hasAttribute("disabled"))!);
    await waitFor(() => expect(mocks.postOperationsRetentionPoliciesIdPreview).toHaveBeenCalledWith({ id: 1 }, { expected_version: 1, batch_size: 100 }));
    expect(screen.getByRole("alertdialog", { name: "确认执行保留批次？" })).toHaveTextContent("找到 2 条候选");
    await user.click(screen.getByRole("button", { name: "确认处理 2 条" }));
    await waitFor(() => expect(mocks.postOperationsRetentionPoliciesIdRun).toHaveBeenCalledWith({ id: 1 }, { expected_version: 1, batch_size: 100 }));
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
