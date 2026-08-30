import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import MonitorsPage from "@/app/dashboard/settings/page";
import { HotKeyAPIError } from "@/lib/request";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getMonitors: vi.fn(),
  getSourceConnections: vi.fn(),
  getMonitorsIdScans: vi.fn(),
  postMonitors: vi.fn(),
  putMonitorsId: vi.fn(),
  putMonitorsIdDraft: vi.fn(),
  getMonitorsIdVersions: vi.fn(),
  postMonitorsIdPublish: vi.fn(),
  getMonitorsIdDraft: vi.fn(),
  putMonitorsIdDraftIntent: vi.fn(),
  postMonitorsIdDraftPreviewRuns: vi.fn(),
  getMonitorsIdDraftPreviewRunsRunId: vi.fn(),
  postMonitorsIdCollect: vi.fn(),
  postMonitorsIdPause: vi.fn(),
  postMonitorsIdResume: vi.fn(),
  postMonitorsIdArchive: vi.fn(),
  postMonitorsIdRestore: vi.fn(),
  deleteMonitorsId: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/monitors", () => ({
  getMonitors: mocks.getMonitors,
  postMonitors: mocks.postMonitors,
  putMonitorsId: mocks.putMonitorsId,
  putMonitorsIdDraft: mocks.putMonitorsIdDraft,
  getMonitorsIdVersions: mocks.getMonitorsIdVersions,
  postMonitorsIdPublish: mocks.postMonitorsIdPublish,
  postMonitorsIdPause: mocks.postMonitorsIdPause,
  postMonitorsIdResume: mocks.postMonitorsIdResume,
  postMonitorsIdArchive: mocks.postMonitorsIdArchive,
  postMonitorsIdRestore: mocks.postMonitorsIdRestore,
  deleteMonitorsId: mocks.deleteMonitorsId,
}));
vi.mock("@/services/hotkey/hotkey-server/monitorIntent", () => ({
  getMonitorsIdDraft: mocks.getMonitorsIdDraft,
  putMonitorsIdDraftIntent: mocks.putMonitorsIdDraftIntent,
  postMonitorsIdDraftPreviewRuns: mocks.postMonitorsIdDraftPreviewRuns,
  getMonitorsIdDraftPreviewRunsRunId:
    mocks.getMonitorsIdDraftPreviewRunsRunId,
}));
vi.mock("@/services/hotkey/hotkey-server/collectionRuns", () => ({
  getMonitorsIdScans: mocks.getMonitorsIdScans,
  postMonitorsIdCollect: mocks.postMonitorsIdCollect,
}));
vi.mock("@/services/hotkey/hotkey-server/sources", () => ({
  getSourceConnections: mocks.getSourceConnections,
}));

const monitor = {
  id: 9,
  version: 3,
  created_by_user_id: 7,
  name: "Claude",
  status: "active",
  query: "Claude",
  collection_interval_seconds: 1800,
  alert_email_enabled: false,
  sources: [
    {
      source_connection_id: 4,
      name: "Hacker News",
      source_type: "hacker_news",
      enabled: true,
    },
  ],
} satisfies HotKeyAPI.MonitorResponse;

describe("MonitorsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAuthStore.setState({
      user: { role: "admin" } as HotKeyAPI.UserResponse,
    });
    mocks.getMonitors.mockResolvedValue({ data: { items: [monitor] } });
    mocks.getSourceConnections.mockResolvedValue({
      data: {
        items: [
          {
            id: 4,
            name: "Hacker News",
            source_type: "hacker_news",
            enabled: true,
            deleted: false,
          },
        ],
      },
    });
    mocks.getMonitorsIdScans.mockResolvedValue({
      data: {
        items: [
          {
            id: "manual:1786622400000000000",
            monitor_id: 9,
            trigger_type: "manual",
            status: "succeeded",
            candidate_count: 8,
            accepted_count: 3,
            rejected_count: 5,
            scheduled_at: "2026-08-13T12:00:00Z",
            finished_at: "2026-08-13T12:00:04Z",
            sources: [
              {
                run_id: 31,
                source_connection_id: 4,
                source_name: "Hacker News",
                source_type: "hacker_news",
                trigger_type: "manual",
                status: "succeeded",
                candidate_count: 8,
                accepted_count: 3,
                rejected_count: 5,
                scheduled_at: "2026-08-13T12:00:00Z",
                finished_at: "2026-08-13T12:00:04Z",
              },
            ],
          },
        ],
      },
    });
    mocks.postMonitors.mockResolvedValue({
      data: { id: 10, version: 2, status: "active" },
    });
    mocks.putMonitorsId.mockResolvedValue({
      data: { id: 9, version: 4, status: "active" },
    });
    mocks.putMonitorsIdDraft.mockImplementation(({ id }: { id: number }) =>
      Promise.resolve({
        data: { id, version: id === 10 ? 3 : 4, status: "active" },
      })
    );
    mocks.getMonitorsIdVersions.mockImplementation(() =>
      Promise.resolve({
        data: {
          items:
            mocks.getMonitorsIdVersions.mock.calls.length === 1
              ? [{ id: 40, version: 2, state: "published" }]
              : [{ id: 41, version: 1, state: "draft" }],
        },
      })
    );
    mocks.getMonitorsIdDraft.mockRejectedValue(
      new HotKeyAPIError(404, "监控意图尚未初始化")
    );
    mocks.putMonitorsIdDraftIntent.mockResolvedValue({
      data: { monitor_id: 10, draft_id: 51, resource_version: 1 },
    });
    mocks.postMonitorsIdDraftPreviewRuns.mockResolvedValue({
      data: { monitor_id: 10, draft_id: 51, resource_version: 1, run_id: 61, status: "queued" },
    });
    mocks.getMonitorsIdDraftPreviewRunsRunId.mockResolvedValue({
      data: { monitor_id: 10, draft_id: 51, resource_version: 1, run_id: 61, status: "succeeded" },
    });
    mocks.postMonitorsIdPublish.mockResolvedValue({
      data: { id: 10, version: 4, status: "active" },
    });
    mocks.postMonitorsIdPause.mockResolvedValue({ data: {} });
    mocks.postMonitorsIdResume.mockResolvedValue({ data: {} });
    mocks.postMonitorsIdArchive.mockResolvedValue({ data: {} });
    mocks.postMonitorsIdRestore.mockResolvedValue({ data: {} });
    mocks.deleteMonitorsId.mockResolvedValue({ data: {} });
    mocks.postMonitorsIdCollect.mockResolvedValue({
      data: {
        requested: 1,
        created: 1,
        reused: 0,
        cooldown_until: "2026-08-13T12:05:00Z",
      },
    });
  });

  it("shows the monitor query and recent source scan without draft or semantic controls", async () => {
    render(<MonitorsPage />);

    expect(
      await screen.findByRole("heading", { name: "Claude" })
    ).toBeInTheDocument();
    expect(screen.getByText("监控词：Claude")).toBeInTheDocument();
    expect(screen.getByText("成功 · 接受 3 / 候选 8")).toBeInTheDocument();
    expect(screen.getByText("Hacker News")).toBeInTheDocument();
	await waitFor(() =>
		expect(mocks.getMonitorsIdScans).toHaveBeenCalledWith({ id: 9, limit: 1 })
	);
    expect(
      screen.queryByText(/草稿|语义意图|AI 候选|配置哈希/)
    ).not.toBeInTheDocument();
  });

  it("shows an accessible loading state while the monitor request is pending", () => {
    mocks.getMonitors.mockReturnValue(new Promise(() => undefined));
    render(<MonitorsPage />);

    expect(
      screen.getByRole("status", { name: "正在加载监控" })
    ).toBeInTheDocument();
  });

  it("shows an explicit empty state without management actions for a viewer", async () => {
    useAuthStore.setState({
      user: { role: "viewer" } as HotKeyAPI.UserResponse,
    });
    mocks.getMonitors.mockResolvedValueOnce({ data: { items: [] } });
    render(<MonitorsPage />);

    expect(await screen.findByText("还没有热点监控")).toBeInTheDocument();
    expect(screen.getByText("当前没有可查看的监控。")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "新建监控" })).not.toBeInTheDocument();
  });

  it("keeps viewer read-only and grants contributor controls to an editor", async () => {
    useAuthStore.setState({
      user: { role: "viewer" } as HotKeyAPI.UserResponse,
    });
    const { unmount } = render(<MonitorsPage />);
    expect(await screen.findByRole("heading", { name: "Claude" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "立即扫描 Claude" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "编辑 Claude" })).not.toBeInTheDocument();
    unmount();

    useAuthStore.setState({
      user: { role: "editor" } as HotKeyAPI.UserResponse,
    });
    render(<MonitorsPage />);
    expect(await screen.findByRole("button", { name: "立即扫描 Claude" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "编辑 Claude" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "暂停" })).toBeInTheDocument();
  });

  it("lets an analyst manage only monitors they own", async () => {
    useAuthStore.setState({
      user: { id: 7, role: "analyst" } as HotKeyAPI.UserResponse,
    });
    mocks.getMonitors.mockResolvedValueOnce({
      data: {
        items: [
          monitor,
          { ...monitor, id: 10, name: "Other", created_by_user_id: 8 },
        ],
      },
    });

    render(<MonitorsPage />);

    expect(await screen.findByRole("button", { name: "新建监控" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "立即扫描 Claude" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "编辑 Claude" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "立即扫描 Other" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "编辑 Other" })).not.toBeInTheDocument();
  });

  it("creates, compiles and publishes a schedulable monitor in one user action", async () => {
    const user = userEvent.setup();
    mocks.getMonitors.mockResolvedValueOnce({ data: { items: [] } });
    render(<MonitorsPage />);

    await user.click(await screen.findByRole("button", { name: "新建监控" }));
    await user.type(screen.getByLabelText("监控名称"), "AI 产品");
    await user.type(screen.getByLabelText("监控词"), "Claude");
    await user.click(screen.getByRole("button", { name: "创建并启用" }));

    await waitFor(() => expect(mocks.postMonitors).toHaveBeenCalledTimes(1));
    const request = mocks.postMonitors.mock.calls[0][0];
    expect(request.name).toBe("AI 产品");
    expect(request.query).toBe("Claude");
    expect(request.source_connection_ids).toEqual([4]);
    expect(request.collection_interval_seconds).toBe(1800);
    expect(request.alert_email_enabled).toBe(true);
    expect(mocks.putMonitorsIdDraft).toHaveBeenCalledWith(
      { id: 10 },
      expect.objectContaining({
        expected_monitor_version: 2,
        expected_draft_version: null,
        name: "AI 产品",
        rules: [
          expect.objectContaining({
            rule_type: "keyword",
            operator: "contains",
            value: "Claude",
          }),
        ],
        sources: [expect.objectContaining({ source_connection_id: 4 })],
      })
    );
    expect(mocks.putMonitorsIdDraftIntent).toHaveBeenCalledWith(
      { id: 10 },
      {
        expected_resource_version: 0,
        objective: "Claude",
        clauses: [{ operator: "should", field: "term", value: "Claude" }],
        entities: [],
        examples: [],
      },
      expect.objectContaining({ headers: { "If-None-Match": "*" } })
    );
    expect(mocks.postMonitorsIdDraftPreviewRuns).toHaveBeenCalledWith(
      { id: 10 },
      {
        expected_resource_version: 1,
        evaluator_profile: "hybrid-preview-v1",
        sample_limit: 25,
      },
      expect.objectContaining({
        headers: expect.objectContaining({
          "If-Match": '"v1"',
          "Idempotency-Key": expect.any(String),
        }),
      })
    );
    expect(mocks.getMonitorsIdDraftPreviewRunsRunId).toHaveBeenCalledWith({
      id: 10,
      run_id: 61,
    });
    expect(mocks.postMonitorsIdPublish).toHaveBeenCalledWith(
      { id: 10 },
      { expected_monitor_version: 3, expected_draft_version: 1 }
    );
  });

  it("keeps the monitor unpublished when the exact intent preview fails", async () => {
    const user = userEvent.setup();
    mocks.getMonitors.mockResolvedValueOnce({ data: { items: [] } });
    mocks.getMonitorsIdDraftPreviewRunsRunId.mockResolvedValueOnce({
      data: {
        monitor_id: 10,
        draft_id: 51,
        resource_version: 1,
        run_id: 61,
        status: "failed",
        failure_code: "preview_evaluator_unavailable",
      },
    });
    render(<MonitorsPage />);

    await user.click(await screen.findByRole("button", { name: "新建监控" }));
    await user.type(screen.getByLabelText("监控名称"), "AI 产品");
    await user.type(screen.getByLabelText("监控词"), "Claude");
    await user.click(screen.getByRole("button", { name: "创建并启用" }));

    await waitFor(() =>
      expect(mocks.getMonitorsIdDraftPreviewRunsRunId).toHaveBeenCalledTimes(1)
    );
    expect(mocks.postMonitorsIdPublish).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "创建并启用" })).toBeEnabled();
  });

  it("recompiles an edited monitor before publishing without exposing draft configuration", async () => {
    const user = userEvent.setup();
    render(<MonitorsPage />);

    await user.click(
      await screen.findByRole("button", { name: "编辑 Claude" })
    );
    const query = screen.getByLabelText("监控词");
    await user.clear(query);
    await user.type(query, "Claude AI");
    await user.click(screen.getByRole("button", { name: "保存修改" }));

    await waitFor(() =>
      expect(mocks.postMonitorsIdPublish).toHaveBeenCalledTimes(1)
    );
    expect(mocks.putMonitorsIdDraft).toHaveBeenCalledWith(
      { id: 9 },
      expect.objectContaining({
        expected_monitor_version: 3,
        name: "Claude",
        rules: [expect.objectContaining({ value: "Claude AI" })],
        sources: [expect.objectContaining({ source_connection_id: 4 })],
      })
    );
    expect(mocks.putMonitorsIdDraftIntent).toHaveBeenCalledWith(
      { id: 9 },
      expect.objectContaining({ objective: "Claude AI" }),
      expect.objectContaining({ headers: { "If-None-Match": "*" } })
    );
    expect(mocks.postMonitorsIdPublish).toHaveBeenCalledWith(
      { id: 9 },
      { expected_monitor_version: 4, expected_draft_version: 1 }
    );
    expect(screen.queryByText(/草稿|配置哈希/)).not.toBeInTheDocument();
  });

  it("submits an idempotent manual scan and exposes the queued state immediately", async () => {
    const user = userEvent.setup();
    render(<MonitorsPage />);

    await user.click(
      await screen.findByRole("button", { name: "立即扫描 Claude" })
    );

    expect(mocks.postMonitorsIdCollect).toHaveBeenCalledWith({ id: 9 });
    expect(screen.getByText("已排队，等待来源返回")).toBeInTheDocument();
  });

  it("pauses and resumes with the monitor version", async () => {
    const user = userEvent.setup();
    const { unmount } = render(<MonitorsPage />);

    await user.click(await screen.findByRole("button", { name: "暂停" }));
    expect(mocks.postMonitorsIdPause).toHaveBeenCalledWith(
      { id: 9 },
      { expected_monitor_version: 3 }
    );
    unmount();

    mocks.getMonitors.mockResolvedValue({
      data: { items: [{ ...monitor, status: "paused" }] },
    });
    render(<MonitorsPage />);
    await user.click(await screen.findByRole("button", { name: "恢复" }));
    expect(mocks.postMonitorsIdResume).toHaveBeenCalledWith(
      { id: 9 },
      { expected_monitor_version: 3 }
    );
  });

  it("deletes an archived monitor only after explicit confirmation", async () => {
    const user = userEvent.setup();
    mocks.getMonitors.mockResolvedValue({
      data: { items: [{ ...monitor, status: "archived" }] },
    });
    render(<MonitorsPage />);

    await user.click(await screen.findByRole("button", { name: "删除" }));
    expect(mocks.deleteMonitorsId).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "确认删除" }));

    expect(mocks.deleteMonitorsId).toHaveBeenCalledWith(
      { id: 9 },
      { expected_monitor_version: 3 }
    );
  });

  it("shows a retryable load error instead of an empty monitor list", async () => {
    mocks.getMonitors.mockRejectedValue(new Error("network unavailable"));
    render(<MonitorsPage />);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "network unavailable"
    );
    expect(screen.getByRole("button", { name: "重试" })).toBeInTheDocument();
  });

  it("shows a dedicated forbidden state without presenting a retry action", async () => {
    mocks.getMonitors.mockRejectedValue(
      new HotKeyAPIError(403, "当前账号没有执行此操作的权限")
    );
    render(<MonitorsPage />);

    expect(
      await screen.findByRole("alert", { name: "权限不足" })
    ).toHaveTextContent("当前账号没有查看监控与扫描记录的权限");
    expect(screen.queryByRole("button", { name: "重试" })).not.toBeInTheDocument();
  });
});
