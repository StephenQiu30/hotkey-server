import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import KnowledgePage from "@/app/dashboard/knowledge/page";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getKnowledgeDocuments: vi.fn(),
  getKnowledgeProposals: vi.fn(),
  postKnowledgeProposalsIdApply: vi.fn(),
  postKnowledgeProposalsIdApprove: vi.fn(),
  postKnowledgeProposalsIdReject: vi.fn(),
  postKnowledgeReconcile: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/knowledge", () => mocks);

function setRole(role: UserRole) {
  useAuthStore.setState({ status: AuthStatus.Authenticated, user: { id: 1, email: "actor@example.test", role }, error: null });
}

describe("KnowledgePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    setRole(UserRole.Editor);
    mocks.getKnowledgeDocuments.mockResolvedValue({ data: { items: [{ id: 8, version: 1, revisionNo: 0, type: "report", reportID: 41, vaultPath: "reports/8.md", status: "planned" }] } });
    mocks.getKnowledgeProposals.mockResolvedValue({ data: { items: [{ id: 12, version: 1, documentID: 8, baseRevisionNo: 0, status: "pending", reason: "日报发布" }] } });
    mocks.postKnowledgeProposalsIdApprove.mockResolvedValue({ data: { id: 12, version: 2, documentID: 8, status: "approved" } });
    mocks.postKnowledgeProposalsIdApply.mockResolvedValue({ data: { id: 8, version: 2, revisionNo: 1, type: "report", reportID: 41, vaultPath: "reports/8.md", status: "active" } });
    mocks.postKnowledgeReconcile.mockResolvedValue({ data: { scanned: 1, conflict: 0, changed: 0, issues: [] } });
  });

  it("lets an editor approve and atomically apply a fenced Vault proposal", async () => {
    const user = userEvent.setup();
    render(<KnowledgePage />);
    expect(await screen.findByText("reports/8.md")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "批准提案" }));
    await waitFor(() => expect(mocks.postKnowledgeProposalsIdApprove).toHaveBeenCalledWith({ id: 12, version: 1 }));
    await user.click(await screen.findByRole("button", { name: "原子发布到 Vault" }));
    await waitFor(() => expect(mocks.postKnowledgeProposalsIdApply).toHaveBeenCalledWith({ id: 12, version: 2 }));
  });

  it("reserves reconciliation for admins", async () => {
    const { unmount } = render(<KnowledgePage />);
    await screen.findByText("reports/8.md");
    expect(screen.queryByRole("button", { name: "执行 Vault 对账" })).not.toBeInTheDocument();
    unmount();

    setRole(UserRole.Admin);
    const user = userEvent.setup();
    render(<KnowledgePage />);
    await user.click(await screen.findByRole("button", { name: "执行 Vault 对账" }));
    expect(await screen.findByLabelText("Vault 对账结果")).toHaveTextContent("扫描 1 个投影");
  });

  it("uses independent opaque cursors for document and proposal pages", async () => {
    mocks.getKnowledgeDocuments
      .mockResolvedValueOnce({ data: { items: [{ id: 8, vaultPath: "reports/8.md", status: "planned" }], next_cursor: "documents-page-2" } })
      .mockResolvedValueOnce({ data: { items: [{ id: 9, vaultPath: "reports/9.md", status: "active" }] } });
    mocks.getKnowledgeProposals
      .mockResolvedValueOnce({ data: { items: [{ id: 12, documentID: 8, status: "pending" }], next_cursor: "proposals-page-2" } })
      .mockResolvedValueOnce({ data: { items: [{ id: 11, documentID: 9, status: "approved" }] } });
    const user = userEvent.setup();
    render(<KnowledgePage />);

    expect(await screen.findByText("reports/8.md")).toBeInTheDocument();
    const documentCard = screen.getByRole("heading", { name: "当前知识文档" }).closest('[data-slot="card"]') as HTMLElement;
    await user.click(within(documentCard).getByRole("button", { name: "下一页" }));
    expect(await screen.findByText("reports/9.md")).toBeInTheDocument();
    expect(mocks.getKnowledgeDocuments).toHaveBeenNthCalledWith(2, { limit: 10, cursor: "documents-page-2" });

    const proposalCard = screen.getByRole("heading", { name: "发布提案与冲突" }).closest('[data-slot="card"]') as HTMLElement;
    await user.click(within(proposalCard).getByRole("button", { name: "下一页" }));
    expect(await screen.findByText("提案 #11")).toBeInTheDocument();
    expect(mocks.getKnowledgeProposals).toHaveBeenNthCalledWith(2, { limit: 10, cursor: "proposals-page-2" });
  });

  it("shows a permission state without loading governance data for analysts", async () => {
    setRole(UserRole.Analyst);
    render(<KnowledgePage />);
    expect(await screen.findByLabelText("知识投影权限不足")).toBeInTheDocument();
    expect(mocks.getKnowledgeDocuments).not.toHaveBeenCalled();
  });

  it("renders the empty state after the loading state", async () => {
    let resolveDocuments!: (value: unknown) => void;
    mocks.getKnowledgeDocuments.mockReturnValueOnce(new Promise((resolve) => { resolveDocuments = resolve; }));
    mocks.getKnowledgeProposals.mockResolvedValueOnce({ data: { items: [] } });
    render(<KnowledgePage />);
    expect(await screen.findByLabelText("正在加载知识投影")).toBeInTheDocument();
    resolveDocuments({ data: { items: [] } });
    expect(await screen.findByRole("heading", { name: "尚未发布知识" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "没有待处理提案" })).toBeInTheDocument();
  });
});
