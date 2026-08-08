import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AgentTokenManager } from "@/components/agent/AgentTokenManager";

const mocks = vi.hoisted(() => ({
  role: "viewer",
  getAgentTokens: vi.fn(),
  postAgentTokens: vi.fn(),
  postAgentTokensIdRevoke: vi.fn(),
}));

vi.mock("@/stores/authStore", () => ({
  useAuthStore: (selector: (state: { user: { role: string } }) => unknown) =>
    selector({ user: { role: mocks.role } }),
}));

vi.mock("@/services/hotkey/hotkey-server/agentAccess", () => ({
  getAgentTokens: mocks.getAgentTokens,
  postAgentTokens: mocks.postAgentTokens,
  postAgentTokensIdRevoke: mocks.postAgentTokensIdRevoke,
}));

const activeToken = {
  id: 7,
  version: 1,
  name: "研究助手",
  token_prefix: "hk_agent_abcdefghi",
  scopes: ["events.read"],
  expires_at: "2099-09-08T08:00:00Z",
  created_at: "2026-08-08T08:00:00Z",
};

describe("AgentTokenManager", () => {
  beforeEach(() => {
    mocks.role = "viewer";
    mocks.getAgentTokens.mockResolvedValue({ data: [] });
    mocks.postAgentTokens.mockResolvedValue({
      data: { ...activeToken, token: "hk_agent_one_time_secret" },
    });
    mocks.postAgentTokensIdRevoke.mockResolvedValue({
      data: { ...activeToken, version: 2, revoked_at: "2026-08-08T09:00:00Z" },
    });
  });

  it("creates a least-privilege token and removes the one-time secret after acknowledgement", async () => {
    const user = userEvent.setup();
    render(<AgentTokenManager />);

    expect(await screen.findByText("尚未创建 Agent Token")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "创建 Token" }));
    const dialog = screen.getByRole("dialog", { name: "创建 Agent Token" });
    expect(within(dialog).queryByText("执行采集")).not.toBeInTheDocument();

    await user.type(within(dialog).getByLabelText("名称"), "研究助手");
    await user.click(within(dialog).getByRole("checkbox", { name: "事件读取" }));
    await user.click(within(dialog).getByRole("button", { name: "创建" }));

    expect(await screen.findByText("hk_agent_one_time_secret")).toBeInTheDocument();
    expect(mocks.postAgentTokens).toHaveBeenCalledWith({
      name: "研究助手",
      scopes: ["events.read"],
      lifetime_days: 30,
    });
    await user.click(screen.getByRole("button", { name: "我已保存" }));
    expect(screen.queryByText("hk_agent_one_time_secret")).not.toBeInTheDocument();
  });

  it("keeps a failed load recoverable", async () => {
    mocks.getAgentTokens
      .mockRejectedValueOnce(new Error("token service unavailable"))
      .mockResolvedValueOnce({ data: [] });
    const user = userEvent.setup();
    render(<AgentTokenManager />);

    expect(await screen.findByText("无法加载 Agent Token")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "重试" }));
    expect(await screen.findByText("尚未创建 Agent Token")).toBeInTheDocument();
  });

  it("revokes an active token with its current version", async () => {
    mocks.getAgentTokens.mockResolvedValue({ data: [activeToken] });
    const user = userEvent.setup();
    render(<AgentTokenManager />);

    expect(await screen.findByText("研究助手")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "撤销" }));
    const confirmation = screen.getByRole("alertdialog");
    await user.click(within(confirmation).getByRole("button", { name: "确认撤销" }));

    expect(mocks.postAgentTokensIdRevoke).toHaveBeenCalledWith(
      { id: 7 },
      { expected_version: 1 },
    );
    expect(await screen.findByText("已撤销")).toBeInTheDocument();
  });

  it("shows search execution only to editor and admin roles", async () => {
    mocks.role = "editor";
    const user = userEvent.setup();
    render(<AgentTokenManager />);

    await screen.findByText("尚未创建 Agent Token");
    await user.click(screen.getByRole("button", { name: "创建 Token" }));
    expect(screen.getByText("执行采集")).toBeInTheDocument();
  });
});
