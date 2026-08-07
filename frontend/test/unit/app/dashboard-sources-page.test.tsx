import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
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

const setRole = (role: UserRole) => {
  useAuthStore.setState({
    status: AuthStatus.Authenticated,
    user: { id: 1, email: `${role}@example.test`, role },
    error: null,
  });
};

const openCompletedForm = async () => {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "新增来源" }));
  await user.type(screen.getByLabelText("名称"), "Research feed");
  await user.type(
    screen.getByLabelText("接口地址"),
    "https://example.test/feed.xml"
  );
  return user;
};

describe("SourcesPage body storage authorization", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getSourceConnections.mockResolvedValue({ data: { items: [] } });
    mocks.postSourceConnections.mockResolvedValue({ data: { id: 1 } });
    setRole(UserRole.Admin);
  });

  it("submits Feed body storage by default", async () => {
    render(<SourcesPage />);
    const user = await openCompletedForm();

    const checkbox = screen.getByRole("checkbox", {
      name: "保存来源正文/摘要用于归档预览",
    });
    expect(checkbox).toBeChecked();
    expect(
      screen.getByText(
        "只保存来源 Feed 实际提供的正文/摘要，不抓取原网页；启用前确认来源条款。"
      )
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "创建连接" }));
    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          config: expect.objectContaining({ allow_body_storage: true }),
        })
      )
    );
  });

  it("submits compliance, env credential, retention, and quota controls", async () => {
    render(<SourcesPage />);
    const user = await openCompletedForm();

    fireEvent.keyDown(screen.getByLabelText("授权方式"), { key: "ArrowDown" });
    fireEvent.click(screen.getByRole("option", { name: "Bearer Token" }));
    await user.type(
      screen.getByLabelText("凭据环境变量引用"),
      "env:RESEARCH_FEED_TOKEN"
    );
    await user.type(
      screen.getByLabelText("条款与政策地址"),
      "https://example.test/terms"
    );
    await user.click(
      screen.getByRole("checkbox", { name: "需要来源归属标记" })
    );
    await user.clear(screen.getByLabelText("每分钟请求上限"));
    await user.type(screen.getByLabelText("每分钟请求上限"), "30");
    await user.clear(screen.getByLabelText("内容保留天数"));
    await user.type(screen.getByLabelText("内容保留天数"), "90");
    await user.type(screen.getByLabelText("允许语言"), "zh-CN, en");
    await user.type(screen.getByLabelText("允许地区"), "CN, US");
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith({
        auth_type: "bearer",
        credential_ref: "env:RESEARCH_FEED_TOKEN",
        enabled: true,
        endpoint: "https://example.test/feed.xml",
        name: "Research feed",
        source_type: "rss",
        terms_policy_url: "https://example.test/terms",
        config: expect.objectContaining({
          allow_body_storage: true,
          allowed_languages: ["zh-CN", "en"],
          allowed_regions: ["CN", "US"],
          content_retention_days: 90,
          rate_limit_per_minute: 30,
          requires_attribution: true,
        }),
      })
    );
  });

  it("creates X Recent Search disabled with a fixed endpoint and Bearer env reference", async () => {
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "新增来源" }));
    await user.type(screen.getByLabelText("名称"), "X 官方搜索");

    fireEvent.keyDown(screen.getByLabelText("来源类型"), { key: "ArrowDown" });
    fireEvent.click(screen.getByRole("option", { name: "X Recent Search" }));

    expect(screen.getByLabelText("接口地址")).toHaveValue(
      "https://api.x.com/2/tweets/search/recent"
    );
    expect(screen.getByLabelText("接口地址")).toHaveAttribute("readonly");
    expect(screen.getByLabelText("授权方式")).toBeDisabled();
    await user.type(
      screen.getByLabelText("凭据环境变量引用"),
      "env:X_BEARER_TOKEN"
    );
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          auth_type: "bearer",
          credential_ref: "env:X_BEARER_TOKEN",
          enabled: false,
          endpoint: "https://api.x.com/2/tweets/search/recent",
          source_type: "x",
        })
      )
    );
  });

  it("creates Microsoft Foundry Web Search disabled after explicit data-boundary review", async () => {
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "新增来源" }));
    await user.type(screen.getByLabelText("名称"), "Foundry Web Search");

    fireEvent.keyDown(screen.getByLabelText("来源类型"), { key: "ArrowDown" });
    fireEvent.click(
      screen.getByRole("option", { name: "Microsoft Foundry Web Search" })
    );

    expect(screen.getByLabelText("接口地址")).toHaveAttribute(
      "placeholder",
      "https://account.services.ai.azure.com/api/projects/project/toolboxes/web-search/versions/1/mcp?api-version=v1"
    );
    expect(screen.getByLabelText("授权方式")).toBeDisabled();
    expect(screen.getByRole("button", { name: "创建连接" })).toBeDisabled();
    await user.type(
      screen.getByLabelText("接口地址"),
      "https://hotkey.services.ai.azure.com/api/projects/hotkey/toolboxes/web-search/versions/1/mcp?api-version=v1"
    );
    await user.type(
      screen.getByLabelText("凭据环境变量引用"),
      "env:AZURE_FOUNDRY_TOKEN"
    );
    expect(
      screen.getByText(/Microsoft DPA 不适用于该能力/)
    ).toBeInTheDocument();
    expect(screen.getByText(/模型生成的派生摘要和引用/)).toBeInTheDocument();
    expect(
      screen.getByText(/不把它标记为原始网页正文或来源指标/)
    ).toBeInTheDocument();
    await user.click(
      screen.getByRole("checkbox", {
        name: "确认 Grounding 数据边界与额外条款",
      })
    );
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          auth_type: "bearer",
          credential_ref: "env:AZURE_FOUNDRY_TOKEN",
          enabled: false,
          source_type: "bing_grounding",
          config: expect.objectContaining({
            allow_body_storage: true,
            requires_attribution: true,
            max_pages_per_run: 1,
            grounding_data_boundary_approved: true,
          }),
        })
      )
    );
  });

  it("creates an authorized Bilibili account disabled with official fixed fields", async () => {
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "新增来源" }));
    await user.type(screen.getByLabelText("名称"), "Bilibili 官方账号");
    fireEvent.keyDown(screen.getByLabelText("来源类型"), { key: "ArrowDown" });
    fireEvent.click(screen.getByRole("option", { name: "Bilibili 开放平台" }));

    expect(screen.getByLabelText("接口地址")).toHaveValue(
      "https://member.bilibili.com/arcopen/fn"
    );
    expect(screen.getByLabelText("接口地址")).toHaveAttribute("readonly");
    expect(screen.getByLabelText("授权方式")).toBeDisabled();
    expect(screen.getByLabelText("条款与政策地址")).toHaveAttribute("readonly");
    expect(
      screen.getByText(/公共 UID、@账号与主页地址不会被解析或抓取/)
    ).toBeInTheDocument();
    await user.type(
      screen.getByLabelText("授权账号 OpenID"),
      "creator_open_id"
    );
    await user.type(
      screen.getByLabelText("凭据环境变量引用"),
      "env:BILIBILI_OAUTH"
    );
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          auth_type: "oauth2",
          credential_ref: "env:BILIBILI_OAUTH",
          enabled: false,
          endpoint: "https://member.bilibili.com/arcopen/fn",
          source_type: "bilibili",
          terms_policy_url:
            "https://openhome.bilibili.com/agreement/privacy-policy",
          config: expect.objectContaining({
            allow_body_storage: true,
            requires_attribution: true,
            requires_deletion_sync: true,
            bilibili_open_id: "creator_open_id",
          }),
        })
      )
    );
  });

  it("creates an official Weibo keyword source disabled behind capability health", async () => {
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "新增来源" }));
    await user.type(screen.getByLabelText("名称"), "微博官方关键词");
    fireEvent.keyDown(screen.getByLabelText("来源类型"), { key: "ArrowDown" });
    fireEvent.click(screen.getByRole("option", { name: "微博开放平台关键词" }));

    expect(screen.getByLabelText("接口地址")).toHaveValue(
      "https://open.weibo.com/cli/api"
    );
    expect(screen.getByLabelText("接口地址")).toHaveAttribute("readonly");
    expect(screen.getByLabelText("授权方式")).toBeDisabled();
    expect(screen.getByLabelText("条款与政策地址")).toHaveAttribute("readonly");
    expect(screen.getByText(/账号须完成开发者认证/)).toBeInTheDocument();
    expect(
      screen.getByText(/不支持账号时间线、热搜页或网页抓取/)
    ).toBeInTheDocument();
    await user.type(
      screen.getByLabelText("凭据环境变量引用"),
      "env:WEIBO_API_TOKEN"
    );
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          auth_type: "bearer",
          credential_ref: "env:WEIBO_API_TOKEN",
          enabled: false,
          endpoint: "https://open.weibo.com/cli/api",
          source_type: "weibo",
          terms_policy_url:
            "https://open.weibo.com/wiki/%E5%BC%80%E5%8F%91%E8%80%85%E5%8D%8F%E8%AE%AE",
          config: expect.objectContaining({
            allow_body_storage: true,
            requires_attribution: true,
            requires_deletion_sync: true,
          }),
        })
      )
    );
  });

  it("creates Google Agent Search disabled with a regional official contract", async () => {
    render(<SourcesPage />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "新增来源" }));
    await user.type(screen.getByLabelText("名称"), "Google 限定域搜索");
    fireEvent.keyDown(screen.getByLabelText("来源类型"), { key: "ArrowDown" });
    fireEvent.click(
      screen.getByRole("option", { name: "Google Agent Search（限定域）" })
    );

    expect(screen.getByLabelText("接口地址")).toHaveValue(
      "https://discoveryengine.googleapis.com"
    );
    expect(screen.getByLabelText("接口地址")).toHaveAttribute("readonly");
    expect(screen.getByLabelText("授权方式")).toBeDisabled();
    expect(screen.getByLabelText("条款与政策地址")).toHaveValue(
      "https://cloud.google.com/terms"
    );
    expect(screen.getByLabelText("条款与政策地址")).toHaveAttribute(
      "readonly"
    );
    expect(screen.getByRole("button", { name: "创建连接" })).toBeDisabled();
    expect(screen.getByText(/已关闭新客户/)).toBeInTheDocument();
    expect(screen.getByText(/不会降级抓取 Google 搜索页/)).toBeInTheDocument();

    await user.type(
      screen.getByLabelText("ServingConfig 资源名"),
      "projects/hotkey-demo/locations/global/collections/default_collection/dataStores/news/servingConfigs/default_config"
    );
    await user.type(
      screen.getByLabelText("凭据环境变量引用"),
      "env:GOOGLE_AGENT_SEARCH_TOKEN"
    );
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          auth_type: "bearer",
          credential_ref: "env:GOOGLE_AGENT_SEARCH_TOKEN",
          enabled: false,
          endpoint: "https://discoveryengine.googleapis.com",
          source_type: "google_agent_search",
          terms_policy_url: "https://cloud.google.com/terms",
          config: expect.objectContaining({
            allow_body_storage: true,
            requires_attribution: true,
            requires_deletion_sync: false,
            google_location: "global",
            google_serving_config:
              "projects/hotkey-demo/locations/global/collections/default_collection/dataStores/news/servingConfigs/default_config",
          }),
        })
      )
    );
  });

  it("shows safe compliance facts without exposing a credential reference", async () => {
    mocks.getSourceConnections.mockResolvedValue({
      data: {
        items: [
          {
            id: 7,
            name: "Official feed",
            source_type: "rss",
            health_status: "healthy",
            credential_configured: true,
            terms_policy_url: "https://example.test/terms",
            config: { rate_limit_per_minute: 45, content_retention_days: 60 },
          },
        ],
      },
    });

    render(<SourcesPage />);

    expect(
      await screen.findByRole("link", { name: "条款与政策" })
    ).toHaveAttribute("href", "https://example.test/terms");
    expect(screen.getByText("凭据已配置")).toBeInTheDocument();
    expect(screen.getByText("45 req/min · 保留 60 天")).toBeInTheDocument();
    expect(screen.queryByText(/env:/)).not.toBeInTheDocument();
  });

  it("shows Sogou as an authorization-gated capability without executable actions", async () => {
    render(<SourcesPage />);

    expect(await screen.findByText("搜狗授权搜索")).toBeInTheDocument();
    expect(screen.getByText("需要授权")).toBeInTheDocument();
    expect(
      screen.getByText(/不抓取搜索结果页，也不会创建或调度搜狗来源连接/)
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "查看官方开放平台说明" })
    ).toHaveAttribute(
      "href",
      "https://data.open.sogou.com/data-resource/help.html?type=1"
    );
    expect(
      screen.getByRole("button", { name: "授权资料未齐备" })
    ).toBeDisabled();
    expect(
      screen.queryByRole("option", { name: /搜狗/ })
    ).not.toBeInTheDocument();
  });

  it("keeps body storage enabled when a new form is opened", async () => {
    render(<SourcesPage />);
    const user = await openCompletedForm();
    const checkbox = screen.getByRole("checkbox", {
      name: "保存来源正文/摘要用于归档预览",
    });

    await user.click(checkbox);
    expect(checkbox).not.toBeChecked();
    await user.click(screen.getByRole("button", { name: "取消" }));
    await user.click(screen.getByRole("button", { name: "新增来源" }));
    expect(
      screen.getByRole("checkbox", {
        name: "保存来源正文/摘要用于归档预览",
      })
    ).toBeChecked();

    await user.type(screen.getByLabelText("名称"), "Second feed");
    await user.type(
      screen.getByLabelText("接口地址"),
      "https://example.test/second.xml"
    );
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          config: expect.objectContaining({ allow_body_storage: true }),
        })
      )
    );
  });

  it.each([UserRole.Viewer, UserRole.Editor])(
    "keeps source creation and body authorization hidden from %s",
    async (role) => {
      setRole(role);
      render(<SourcesPage />);

      expect(await screen.findByText("只读来源目录")).toBeInTheDocument();
      expect(
        screen.getByText(
          "查看当前工作区已接入的 RSS、Hacker News、X、微博关键词、Bilibili 授权账号、Google Agent Search 与 Microsoft Foundry Web Search。"
        )
      ).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "新增来源" })
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("checkbox", {
          name: "保存来源正文/摘要用于归档预览",
        })
      ).not.toBeInTheDocument();
    }
  );

  it("lets an admin explicitly enable Feed body storage for an existing source", async () => {
    mocks.getSourceConnections
      .mockResolvedValueOnce({
        data: {
          items: [
            {
              id: 3,
              version: 4,
              name: "bioRxiv · Bioinformatics",
              source_type: "rss",
              enabled: true,
              deleted: false,
              config: { allow_body_storage: false },
            },
          ],
        },
      })
      .mockResolvedValueOnce({ data: { items: [] } });

    render(<SourcesPage />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "开启归档" }));

    expect(
      await screen.findByRole("alertdialog", { name: "开启正文与摘要归档？" })
    ).toBeInTheDocument();
    expect(mocks.patchSourceConnectionsId).not.toHaveBeenCalled();
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "确认开启" }));

    await waitFor(() =>
      expect(mocks.patchSourceConnectionsId).toHaveBeenCalledWith(
        { id: 3 },
        {
          expected_source_version: 4,
          config: { allow_body_storage: true },
        }
      )
    );
  });

  it("requests the next source page with the returned cursor", async () => {
    mocks.getSourceConnections
      .mockResolvedValueOnce({
        data: {
          items: [{ id: 3, name: "First source", deleted: false }],
          next_cursor: "source-cursor-1",
        },
      })
      .mockResolvedValueOnce({ data: { items: [], next_cursor: undefined } });

    render(<SourcesPage />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "下一页" }));

    await waitFor(() =>
      expect(mocks.getSourceConnections).toHaveBeenLastCalledWith({
        cursor: "source-cursor-1",
        limit: 20,
      })
    );
  });

  it("keeps a failed source request distinct from the real empty state and retries", async () => {
    mocks.getSourceConnections
      .mockRejectedValueOnce(new Error("来源服务暂时不可用"))
      .mockResolvedValueOnce({
        data: {
          items: [{ id: 9, name: "Hacker News 官方", deleted: false }],
        },
      });

    render(<SourcesPage />);

    expect(await screen.findByText("来源加载失败")).toBeInTheDocument();
    expect(screen.queryByText("还没有来源连接")).not.toBeInTheDocument();

    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "重新加载" }));

    expect(await screen.findByText("Hacker News 官方")).toBeInTheDocument();
    expect(screen.queryByText("来源加载失败")).not.toBeInTheDocument();
  });
});
