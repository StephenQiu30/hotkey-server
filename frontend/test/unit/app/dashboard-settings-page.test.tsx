import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getMonitors: vi.fn(),
  getMonitorsId: vi.fn(),
  getMonitorsIdVersions: vi.fn(),
  postMonitors: vi.fn(),
  putMonitorsIdDraft: vi.fn(),
  postMonitorsIdPreview: vi.fn(),
  postMonitorsIdPublish: vi.fn(),
  postMonitorsIdPause: vi.fn(),
  postMonitorsIdResume: vi.fn(),
  postMonitorsIdArchive: vi.fn(),
  postMonitorsIdDraftAiCandidates: vi.fn(),
  postMonitorsIdDraftRulesRuleIdApproval: vi.fn(),
  postMonitorsIdCollect: vi.fn(),
  postMonitorsIdRestore: vi.fn(),
  deleteMonitorsId: vi.fn(),
  getSourceConnections: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/monitors", () => ({
  getMonitors: mocks.getMonitors,
  getMonitorsId: mocks.getMonitorsId,
  getMonitorsIdVersions: mocks.getMonitorsIdVersions,
  postMonitors: mocks.postMonitors,
  putMonitorsIdDraft: mocks.putMonitorsIdDraft,
  postMonitorsIdPreview: mocks.postMonitorsIdPreview,
  postMonitorsIdPublish: mocks.postMonitorsIdPublish,
  postMonitorsIdPause: mocks.postMonitorsIdPause,
  postMonitorsIdResume: mocks.postMonitorsIdResume,
  postMonitorsIdArchive: mocks.postMonitorsIdArchive,
  postMonitorsIdDraftAiCandidates: mocks.postMonitorsIdDraftAiCandidates,
  postMonitorsIdDraftRulesRuleIdApproval: mocks.postMonitorsIdDraftRulesRuleIdApproval,
  postMonitorsIdRestore: mocks.postMonitorsIdRestore,
  deleteMonitorsId: mocks.deleteMonitorsId,
}));
vi.mock("@/services/hotkey/hotkey-server/sources", () => ({
  getSourceConnections: mocks.getSourceConnections,
}));
vi.mock("@/services/hotkey/hotkey-server/collectionRuns", () => ({
  postMonitorsIdCollect: mocks.postMonitorsIdCollect,
}));

import MonitorsPage from "@/app/dashboard/settings/page";

const draft = {
  id: 11,
  version: 3,
  revision: 2,
  state: "draft",
  timezone: "Asia/Shanghai",
  languages: ["zh"],
  regions: ["CN"],
  collection_interval_seconds: 900,
  relevance_threshold: 70,
  event_threshold: 80,
  retention_days: 30,
  rules: [{ id: 21, rule_type: "keyword", operator: "contains", value: "OpenAI", enabled: true }],
  sources: [{ id: 31, source_connection_id: 7, name: "Official feed", source_type: "rss", enabled: true }],
} satisfies HotKeyAPI.MonitorConfigResponse;

const published = {
  ...draft,
  id: 10,
  version: 2,
  revision: 1,
  state: "published",
  config_hash: "a".repeat(64),
  published_at: "2026-08-07T08:30:00Z",
} satisfies HotKeyAPI.MonitorConfigResponse;

const monitor = {
  id: 1,
  version: 4,
  name: "AI releases",
  description: "Track official launches",
  status: "active",
  published_revision: 1,
  published,
  draft,
} satisfies HotKeyAPI.MonitorResponse;

function setRole(role: UserRole) {
  useAuthStore.setState({
    status: AuthStatus.Authenticated,
    user: { id: 1, email: `${role}@example.test`, role },
    error: null,
  });
}

async function openMonitorActions(user = userEvent.setup()) {
  await user.click(
    await screen.findByRole("button", { name: "AI releases 操作" })
  );
  return user;
}

describe("MonitorsPage", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    setRole(UserRole.Admin);
    mocks.getMonitors.mockResolvedValue({ data: { items: [monitor] } });
    mocks.getSourceConnections.mockResolvedValue({ data: { items: [{ id: 7, name: "Official feed", source_type: "rss", enabled: true, deleted: false }] } });
    mocks.getMonitorsId.mockResolvedValue({ data: monitor });
    mocks.getMonitorsIdVersions.mockResolvedValue({ data: { items: [draft, published] } });
    mocks.putMonitorsIdDraft.mockResolvedValue({ data: monitor });
    mocks.postMonitorsIdPreview.mockResolvedValue({ data: { eligible: true, config_hash: "b".repeat(64), estimated_requests: 2, warnings: [], sources: [{ source_connection_id: 7, estimated_requests: 2, compiled_query: "OpenAI -jobs", query_mode: "local_filter", query_signature: "c".repeat(64), languages: ["zh", "en"], max_query_bytes: 2048, included_term_count: 4, excluded_term_count: 1 }] } });
    mocks.postMonitorsIdPublish.mockResolvedValue({ data: { ...monitor, status: "active", draft: undefined } });
    mocks.postMonitorsIdDraftAiCandidates.mockResolvedValue({ data: { id: 41, origin: "ai", approval_status: "pending" } });
    mocks.postMonitorsIdDraftRulesRuleIdApproval.mockResolvedValue({ data: null });
    mocks.postMonitorsIdPause.mockResolvedValue({ data: { ...monitor, version: 5, status: "paused" } });
    mocks.postMonitorsIdCollect.mockResolvedValue({ data: { requested: 1, created: 1, reused: 0, cooldown_until: "2026-08-07T09:05:00Z" } });
  });

  it("keeps viewer access read-only without requesting source management data", async () => {
    setRole(UserRole.Viewer);
    render(<MonitorsPage />);

    expect(await screen.findByText("AI releases")).toBeInTheDocument();
    expect(screen.getByText("只读监控目录")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "新建监控" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "AI releases 操作" })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "相关性判定" })).toHaveAttribute(
      "href",
      "/dashboard/settings/monitors/1/matches",
    );
    expect(mocks.getSourceConnections).not.toHaveBeenCalled();
  });

  it("guides editors to create a monitor when an enabled source is available", async () => {
    mocks.getMonitors.mockResolvedValue({ data: { items: [] } });

    render(<MonitorsPage />);

    expect(
      await screen.findByText("点击“新建监控”配置规则并选择已启用来源。"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("至少需要一个已启用来源才能创建监控。"),
    ).not.toBeInTheDocument();
  });

  it("lets an editor update a draft but never exposes publication or lifecycle controls", async () => {
    setRole(UserRole.Editor);
    render(<MonitorsPage />);
    const user = userEvent.setup();

    await openMonitorActions(user);
    await user.click(screen.getByRole("menuitem", { name: "编辑草稿" }));
    expect(screen.getByRole("dialog", { name: "编辑监控草稿" })).toBeInTheDocument();
    expect(screen.getByLabelText("规则 1 内容")).toHaveValue("OpenAI");
    await user.click(screen.getByRole("button", { name: "保存草稿" }));

    await waitFor(() => expect(mocks.putMonitorsIdDraft).toHaveBeenCalledWith(
      { id: 1 },
      expect.objectContaining({ expected_monitor_version: 4, expected_draft_version: 3, name: "AI releases" }),
    ));
    await openMonitorActions(user);
    expect(
      screen.getByRole("menuitem", { name: "编辑语义意图" })
    ).toHaveAttribute("href", "/dashboard/settings/monitors/1/intent");
    expect(screen.queryByRole("menuitem", { name: "预览并发布" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "暂停" })).not.toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "立即搜索" })).toBeInTheDocument();
  });

  it("submits an active published monitor through the manual collection API", async () => {
    setRole(UserRole.Editor);
    render(<MonitorsPage />);

    const user = await openMonitorActions();
    await user.click(screen.getByRole("menuitem", { name: "立即搜索" }));

    await waitFor(() => expect(mocks.postMonitorsIdCollect).toHaveBeenCalledWith({ id: 1 }));
    expect(screen.queryByLabelText("手动搜索配额")).not.toBeInTheDocument();
  });

  it("requires a successful preview before an admin can publish the exact draft", async () => {
    render(<MonitorsPage />);
    const user = userEvent.setup();

    await openMonitorActions(user);
    await user.click(screen.getByRole("menuitem", { name: "预览并发布" }));
    expect(mocks.postMonitorsIdPreview).toHaveBeenCalledWith({ id: 1 });
    expect(await screen.findByRole("dialog", { name: "发布预览" })).toBeInTheDocument();
    expect(screen.getByText("预计请求 2 次")).toBeInTheDocument();
    expect(screen.getByText("OpenAI -jobs")).toBeInTheDocument();
    expect(screen.getByText("local_filter")).toBeInTheDocument();
    expect(screen.getByText("包含 4")).toBeInTheDocument();
    expect(screen.getByText("排除 1")).toBeInTheDocument();
    expect(mocks.postMonitorsIdPublish).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "确认发布" }));

    await waitFor(() => expect(mocks.postMonitorsIdPublish).toHaveBeenCalledWith(
      { id: 1 },
      { expected_monitor_version: 4, expected_draft_version: 3 },
    ));
  });

  it("shows lifecycle, schedule, rules, sources, and immutable version history in details", async () => {
    render(<MonitorsPage />);
    await userEvent.setup().click(await screen.findByRole("button", { name: "查看详情" }));

    expect(await screen.findByRole("dialog", { name: "监控详情" })).toBeInTheDocument();
    expect(mocks.getMonitorsId).toHaveBeenCalledWith({ id: 1 });
    expect(mocks.getMonitorsIdVersions).toHaveBeenCalledWith({ id: 1 });
    expect(screen.getByText("每 15 分钟采集")).toBeInTheDocument();
    expect(screen.getByText("Official feed")).toBeInTheDocument();
    expect(screen.getAllByText("OpenAI").length).toBeGreaterThan(0);
    expect(screen.getAllByText("修订 2").length).toBeGreaterThan(0);
    expect(screen.getByText("修订 1")).toBeInTheDocument();
  });

  it("keeps AI expansion candidates pending until an administrator approves them", async () => {
    render(<MonitorsPage />);
    const user = userEvent.setup();
    await openMonitorActions(user);
    await user.click(screen.getByRole("menuitem", { name: "导入 AI 候选" }));
    await user.type(screen.getByLabelText("候选内容"), "agentic workflow");
    await user.click(screen.getByRole("button", { name: "加入待审批" }));
    await waitFor(() => expect(mocks.postMonitorsIdDraftAiCandidates).toHaveBeenCalledWith(
      { id: 1 },
      expect.objectContaining({ expected_monitor_version: 4, expected_draft_version: 3, value: "agentic workflow", weight: 60 }),
    ));

    const pending = { ...draft, rules: [...(draft.rules ?? []), { id: 41, rule_type: "phrase", operator: "contains", value: "agentic workflow", origin: "ai", approval_status: "pending", weight: 60, enabled: true }] };
    mocks.getMonitorsId.mockResolvedValue({ data: { ...monitor, draft: pending } });
    await user.click(screen.getByRole("button", { name: "查看详情" }));
    await user.click(await screen.findByRole("button", { name: "批准" }));
    await waitFor(() => expect(mocks.postMonitorsIdDraftRulesRuleIdApproval).toHaveBeenCalledWith(
      { id: 1, rule_id: 41 },
      { approval: "approved", expected_monitor_version: 4, expected_draft_version: 3 },
    ));
  });

  it("supports direct lifecycle actions and a visible retry state", async () => {
    render(<MonitorsPage />);
    const user = userEvent.setup();
    await openMonitorActions(user);
    await user.click(screen.getByRole("menuitem", { name: "暂停" }));
    await waitFor(() => expect(mocks.postMonitorsIdPause).toHaveBeenCalledWith(
      { id: 1 },
      { expected_monitor_version: 4 },
    ));

    cleanup();
    mocks.getMonitors.mockRejectedValueOnce(new Error("monitor network unavailable"));
    render(<MonitorsPage />);
    expect(await screen.findByText("monitor network unavailable")).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "重试" }));
    expect(await screen.findByText("AI releases")).toBeInTheDocument();
  });
});
