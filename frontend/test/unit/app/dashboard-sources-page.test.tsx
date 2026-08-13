import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SourcesPage from "@/app/dashboard/sources/page";
import { useAuthStore } from "@/stores/authStore";
import { AuthStatus, UserRole } from "@/lib/domainEnums";

const mocks = vi.hoisted(() => ({
  getSourceConnections: vi.fn(),
  getSourcePresets: vi.fn(),
  postSourceConnections: vi.fn(),
  patchSourceConnectionsId: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/sources", () => ({
  getSourceConnections: mocks.getSourceConnections,
  getSourcePresets: mocks.getSourcePresets,
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

const chooseOption = async (
  user: ReturnType<typeof userEvent.setup>,
  label: string,
  option: string,
) => {
  const select = screen.getByRole("combobox", { name: label });
  await waitFor(() => expect(select).toBeEnabled());
  await user.click(select);
  await user.click(screen.getByRole("option", { name: option }));
};

const sourcePresets: HotKeyAPI.SourcePresetResponse[] = [
  { id: "rss_custom", label: "RSS / Atom 地址", source_type: "rss", auth_label: "无需授权", cost: "free", inputs: [{ key: "endpoint", label: "Feed 地址", placeholder: "https://example.com/feed.xml", required: true, max_length: 2048 }] },
  { id: "youtube_channel", label: "YouTube 频道（免费）", source_type: "rss", auth_label: "无需密钥", cost: "free", inputs: [{ key: "youtube_channel_id", label: "YouTube 频道 ID", required: true, max_length: 24 }] },
  { id: "github_releases", label: "GitHub Releases（免费）", source_type: "rss", auth_label: "无需密钥", cost: "free", inputs: [{ key: "github_repository", label: "GitHub 仓库", required: true, max_length: 140 }] },
  { id: "arxiv_search", label: "arXiv 关键词（免费）", source_type: "rss", auth_label: "无需密钥", cost: "free", inputs: [{ key: "arxiv_query", label: "arXiv 关键词", required: true, max_length: 200 }] },
  { id: "mastodon_account", label: "Mastodon 账号（免费）", source_type: "rss", auth_label: "无需密钥", cost: "free", inputs: [{ key: "mastodon_instance", label: "Mastodon 实例", required: true }, { key: "mastodon_value", label: "Mastodon 用户名", required: true }] },
  { id: "mastodon_hashtag", label: "Mastodon 标签（免费）", source_type: "rss", auth_label: "无需密钥", cost: "free", inputs: [{ key: "mastodon_instance", label: "Mastodon 实例", required: true }, { key: "mastodon_value", label: "Mastodon 标签", required: true }] },
  { id: "hacker_news", label: "Hacker News", source_type: "hacker_news", auth_label: "无需授权", cost: "free", inputs: [] },
  { id: "x", label: "X / Twitter（官方付费）", source_type: "x", auth_label: "Bearer Token", cost: "paid", credential_required: true, inputs: [] },
  { id: "bing_grounding", label: "Bing Grounding", source_type: "bing_grounding", auth_label: "Bearer Token", cost: "credentialed", credential_required: true, inputs: [] },
  { id: "bilibili", label: "Bilibili", source_type: "bilibili", auth_label: "OAuth 2.0", cost: "credentialed", credential_required: true, inputs: [] },
  { id: "weibo", label: "Weibo", source_type: "weibo", auth_label: "Bearer Token", cost: "credentialed", credential_required: true, inputs: [] },
  { id: "google_agent_search", label: "Google Agent Search", source_type: "google_agent_search", auth_label: "Bearer Token", cost: "credentialed", credential_required: true, inputs: [] },
];

describe("multi-source workspace", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getSourceConnections.mockResolvedValue({ data: { items: [] } });
    mocks.getSourcePresets.mockResolvedValue({ data: { items: sourcePresets } });
    mocks.postSourceConnections.mockResolvedValue({ data: { id: 1 } });
    setRole(UserRole.Admin);
  });

  it("offers free feed presets before paid or credentialed providers", async () => {
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "新增来源" }));
    const sourcePreset = screen.getByRole("combobox", { name: "接入方式" });
    await waitFor(() => expect(sourcePreset).toBeEnabled());
    expect(sourcePreset.tagName).toBe("BUTTON");
    await user.click(sourcePreset);
    expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual([
      "RSS / Atom 地址",
      "YouTube 频道（免费）",
      "GitHub Releases（免费）",
      "arXiv 关键词（免费）",
      "Mastodon 账号（免费）",
      "Mastodon 标签（免费）",
      "Hacker News",
      "X / Twitter（官方付费）",
      "Bing Grounding",
      "Bilibili",
      "Weibo",
      "Google Agent Search",
    ]);
    await user.keyboard("{Escape}");
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
    await chooseOption(user, "接入方式", "X / Twitter（官方付费）");
    expect(screen.getByText("Bearer Token · 官方付费")).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "启用 X 持续指标刷新" })).not.toBeInTheDocument();

    await user.type(screen.getByLabelText("名称"), "X 官方热点");
    await user.type(screen.getByLabelText("访问凭据"), "x-bearer-token");
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          preset_id: "x",
          preset_values: [],
          credential: "x-bearer-token",
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
    await chooseOption(user, "接入方式", "X / Twitter（官方付费）");
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
    await chooseOption(user, "接入方式", "RSS / Atom 地址");
    await user.type(screen.getByLabelText("名称"), "OpenAI Feed");
    await user.type(await screen.findByLabelText("Feed 地址"), "https://example.test/feed.xml");
    await user.click(screen.getByRole("button", { name: "创建连接" }));
    await waitFor(() => expect(mocks.postSourceConnections).toHaveBeenCalledWith(expect.objectContaining({
      preset_id: "rss_custom",
      preset_values: [{ key: "endpoint", value: "https://example.test/feed.xml" }],
    })));
  });

  it.each([
    {
      preset: "YouTube 频道（免费）",
      fields: [["YouTube 频道 ID", "UC_x5XG1OV2P6uZZ5FSM9Ttw"]],
      presetID: "youtube_channel",
    },
    {
      preset: "GitHub Releases（免费）",
      fields: [["GitHub 仓库", "openai/openai-node"]],
      presetID: "github_releases",
    },
    {
      preset: "arXiv 关键词（免费）",
      fields: [["arXiv 关键词", "graph neural networks"]],
      presetID: "arxiv_search",
    },
    {
      preset: "Mastodon 账号（免费）",
      fields: [["Mastodon 实例", "mastodon.social"], ["Mastodon 用户名", "Gargron"]],
      presetID: "mastodon_account",
    },
    {
      preset: "Mastodon 标签（免费）",
      fields: [["Mastodon 实例", "mastodon.social"], ["Mastodon 标签", "opensource"]],
      presetID: "mastodon_hashtag",
    },
  ])("submits only the $preset choice and its values", async ({ preset, fields, presetID }) => {
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "新增来源" }));
    await chooseOption(user, "接入方式", preset);
    await user.type(screen.getByLabelText("名称"), preset);
    for (const [label, value] of fields) {
      await user.type(screen.getByLabelText(label), value);
    }
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() => expect(mocks.postSourceConnections).toHaveBeenCalledWith(expect.objectContaining({
      preset_id: presetID,
      preset_values: fields.map(([key, value]) => ({
        key: sourcePresets.find((item) => item.id === presetID)?.inputs?.find((input) => input.label === key)?.key,
        value,
      })),
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
