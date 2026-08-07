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
    "https://example.test/feed.xml",
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
        "只保存来源 Feed 实际提供的正文/摘要，不抓取原网页；启用前确认来源条款。",
      ),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "创建连接" }));
    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          config: expect.objectContaining({ allow_body_storage: true }),
        }),
      ),
    );
  });

  it("submits compliance, env credential, retention, and quota controls", async () => {
    render(<SourcesPage />);
    const user = await openCompletedForm();

    fireEvent.keyDown(screen.getByLabelText("授权方式"), { key: "ArrowDown" });
    fireEvent.click(screen.getByRole("option", { name: "Bearer Token" }));
    await user.type(screen.getByLabelText("凭据环境变量引用"), "env:RESEARCH_FEED_TOKEN");
    await user.type(screen.getByLabelText("条款与政策地址"), "https://example.test/terms");
    await user.click(screen.getByRole("checkbox", { name: "需要来源归属标记" }));
    await user.clear(screen.getByLabelText("每分钟请求上限"));
    await user.type(screen.getByLabelText("每分钟请求上限"), "30");
    await user.clear(screen.getByLabelText("内容保留天数"));
    await user.type(screen.getByLabelText("内容保留天数"), "90");
    await user.type(screen.getByLabelText("允许语言"), "zh-CN, en");
    await user.type(screen.getByLabelText("允许地区"), "CN, US");
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() => expect(mocks.postSourceConnections).toHaveBeenCalledWith({
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
    }));
  });

  it("shows safe compliance facts without exposing a credential reference", async () => {
    mocks.getSourceConnections.mockResolvedValue({ data: { items: [{
      id: 7,
      name: "Official feed",
      source_type: "rss",
      health_status: "healthy",
      credential_configured: true,
      terms_policy_url: "https://example.test/terms",
      config: { rate_limit_per_minute: 45, content_retention_days: 60 },
    }] } });

    render(<SourcesPage />);

    expect(await screen.findByRole("link", { name: "条款与政策" })).toHaveAttribute("href", "https://example.test/terms");
    expect(screen.getByText("凭据已配置")).toBeInTheDocument();
    expect(screen.getByText("45 req/min · 保留 60 天")).toBeInTheDocument();
    expect(screen.queryByText(/env:/)).not.toBeInTheDocument();
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
      }),
    ).toBeChecked();

    await user.type(screen.getByLabelText("名称"), "Second feed");
    await user.type(
      screen.getByLabelText("接口地址"),
      "https://example.test/second.xml",
    );
    await user.click(screen.getByRole("button", { name: "创建连接" }));

    await waitFor(() =>
      expect(mocks.postSourceConnections).toHaveBeenCalledWith(
        expect.objectContaining({
          config: expect.objectContaining({ allow_body_storage: true }),
        }),
      ),
    );
  });

  it.each([UserRole.Viewer, UserRole.Editor])(
    "keeps source creation and body authorization hidden from %s",
    async (role) => {
      setRole(role);
      render(<SourcesPage />);

      expect(
        await screen.findByText("只读来源目录"),
      ).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "新增来源" })).not.toBeInTheDocument();
      expect(
        screen.queryByRole("checkbox", {
          name: "保存来源正文/摘要用于归档预览",
        }),
      ).not.toBeInTheDocument();
    },
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
    await userEvent.setup().click(await screen.findByRole("button", { name: "开启归档" }));

    expect(
      await screen.findByRole("alertdialog", { name: "开启正文与摘要归档？" }),
    ).toBeInTheDocument();
    expect(mocks.patchSourceConnectionsId).not.toHaveBeenCalled();
    await userEvent.setup().click(screen.getByRole("button", { name: "确认开启" }));

    await waitFor(() =>
      expect(mocks.patchSourceConnectionsId).toHaveBeenCalledWith(
        { id: 3 },
        {
          expected_source_version: 4,
          config: { allow_body_storage: true },
        },
      ),
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
    await userEvent.setup().click(await screen.findByRole("button", { name: "下一页" }));

    await waitFor(() =>
      expect(mocks.getSourceConnections).toHaveBeenLastCalledWith({
        cursor: "source-cursor-1",
        limit: 20,
      }),
    );
  });
});
