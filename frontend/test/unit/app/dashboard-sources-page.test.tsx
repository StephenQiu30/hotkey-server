import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SourcesPage from "@/app/dashboard/sources/page";
import { useAuthStore } from "@/stores/authStore";
import { AuthStatus, UserRole } from "@/lib/domainEnums";

const mocks = vi.hoisted(() => ({
  getSourceConnections: vi.fn(),
  postSourceConnections: vi.fn(),
  patchSourceConnectionsId: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/sources", () => ({
  getSourceConnections: mocks.getSourceConnections,
  postSourceConnections: mocks.postSourceConnections,
  patchSourceConnectionsId: mocks.patchSourceConnectionsId,
  postSourceConnectionsIdDisable: vi.fn(),
  postSourceConnectionsIdEnable: vi.fn(),
  postSourceConnectionsIdHealth: vi.fn(),
  postSourceConnectionsIdArchive: vi.fn(),
}));

const setRole = (role: UserRole) =>
  useAuthStore.setState({
    status: AuthStatus.Authenticated,
    user: { id: 1, email: `${role}@example.test`, role },
    error: null,
  });

describe("multi-source workspace", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getSourceConnections.mockResolvedValue({ data: { items: [] } });
    mocks.postSourceConnections.mockResolvedValue({ data: { id: 1 } });
    setRole(UserRole.Admin);
  });

  it("offers all seven supported source types", async () => {
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "新增来源" }));
    const sourceType = screen.getByRole("combobox", { name: "来源类型" });
    expect(Array.from((sourceType as HTMLSelectElement).options).map((option) => option.text)).toEqual([
      "RSS / Atom",
      "Hacker News",
      "X / Twitter",
      "Bing Grounding",
      "Bilibili",
      "Weibo",
      "Google Agent Search",
    ]);
    expect(screen.queryByLabelText("允许语言")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "高级设置" })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  it("creates an official X source with continuous metrics disabled by default", async () => {
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "新增来源" }));
    fireEvent.change(screen.getByRole("combobox", { name: "来源类型" }), { target: { value: "x" } });
    expect(screen.getByText("官方接口 · Bearer Token")).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "启用 X 持续指标刷新" })).not.toBeInTheDocument();

    await user.type(screen.getByLabelText("名称"), "X 官方热点");
    await user.type(screen.getByLabelText("访问凭据"), "x-bearer-token");
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          source_type: "x",
          auth_type: "bearer",
          credential: "x-bearer-token",
          enabled: false,
          endpoint: "https://api.x.com/2/tweets/search/recent",
          config: expect.objectContaining({
            x_metric_refresh_enabled: false,
            x_metric_refresh_interval_minutes: 60,
            x_metric_refresh_observation_hours: 48,
            x_metric_refresh_max_posts_per_run: 100,
            x_metric_refresh_daily_request_budget: 24,
          }),
        })
      )
    );
  });

  it("submits bounded X refresh configuration and locale filters", async () => {
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "新增来源" }));
    fireEvent.change(screen.getByRole("combobox", { name: "来源类型" }), { target: { value: "x" } });
    await user.type(screen.getByLabelText("名称"), "Research X");
    await user.type(screen.getByLabelText("访问凭据"), "secret");
    await user.click(screen.getByRole("button", { name: "高级设置" }));
    await user.click(screen.getByRole("checkbox", { name: "启用 X 持续指标刷新" }));
    fireEvent.change(screen.getByLabelText("允许语言"), { target: { value: "zh-CN, en" } });
    fireEvent.change(screen.getByLabelText("允许地区"), { target: { value: "CN, US" } });
    fireEvent.change(screen.getByLabelText("刷新间隔（分钟）"), { target: { value: "30" } });
    fireEvent.change(screen.getByLabelText("持续观察期（小时）"), { target: { value: "72" } });
    fireEvent.change(screen.getByLabelText("单轮最多 Post"), { target: { value: "50" } });
    fireEvent.change(screen.getByLabelText("每日批次预算"), { target: { value: "12" } });
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          config: expect.objectContaining({
            allowed_languages: ["zh-CN", "en"],
            allowed_regions: ["CN", "US"],
            x_metric_refresh_enabled: true,
            x_metric_refresh_interval_minutes: 30,
            x_metric_refresh_observation_hours: 72,
            x_metric_refresh_max_posts_per_run: 50,
            x_metric_refresh_daily_request_budget: 12,
          }),
        })
      )
    );
  });

  it("creates an RSS source without a credential", async () => {
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "新增来源" }));
    await user.type(screen.getByLabelText("名称"), "OpenAI Feed");
    await user.type(screen.getByLabelText("接口地址"), "https://example.test/feed.xml");
    await user.click(screen.getByRole("button", { name: "创建连接" }));
    await waitFor(() => expect(mocks.postSourceConnections).toHaveBeenCalledWith(expect.objectContaining({
      source_type: "rss",
      auth_type: "none",
      endpoint: "https://example.test/feed.xml",
    })));
  });

  it.each([UserRole.Viewer, UserRole.Editor])(
    "does not render or load source management for %s",
    (role) => {
      setRole(role);
      const { container } = render(<SourcesPage />);

      expect(container).toBeEmptyDOMElement();
      expect(mocks.getSourceConnections).not.toHaveBeenCalled();
    },
  );

  it("replaces an existing X credential without reading the old value", async () => {
    mocks.getSourceConnections.mockResolvedValue({
      data: { items: [{ id: 7, version: 3, name: "Official X", source_type: "x", enabled: false, credential_configured: true, config: { allow_body_storage: true } }] },
    });
    mocks.patchSourceConnectionsId.mockResolvedValue({ data: { id: 7 } });
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "替换凭据" }));
    await user.type(screen.getByLabelText("新访问凭据"), "rotated-token");
    await user.click(screen.getByRole("button", { name: "保存并替换" }));
    await waitFor(() =>
      expect(mocks.patchSourceConnectionsId).toHaveBeenCalledWith(
        { id: 7 },
        { expected_source_version: 3, credential: "rotated-token" }
      )
    );
  });

  it("requests the next source page with the returned cursor", async () => {
    mocks.getSourceConnections
      .mockResolvedValueOnce({ data: { items: [{ id: 3, name: "First X", deleted: false }], next_cursor: "source-cursor-1" } })
      .mockResolvedValueOnce({ data: { items: [] } });
    render(<SourcesPage />);
    await userEvent.setup().click(await screen.findByRole("button", { name: "下一页" }));
    await waitFor(() =>
      expect(mocks.getSourceConnections).toHaveBeenLastCalledWith({ cursor: "source-cursor-1", limit: 20 })
    );
  });
});
