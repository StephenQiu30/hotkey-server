import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ReportsPage from "@/app/dashboard/reports/page";
import { KnowledgeArchive } from "@/components/reports/KnowledgeArchive";

const mocks = vi.hoisted(() => ({
  getReports: vi.fn(), postReports: vi.fn(), preview: vi.fn(), build: vi.fn(), publish: vi.fn(),
  getSubscriptions: vi.fn(), getDocuments: vi.fn(), getProposals: vi.fn(), approve: vi.fn(), reject: vi.fn(), apply: vi.fn(), reconcile: vi.fn(),
}));

vi.mock("@/stores/authStore", () => ({ useAuthStore: (selector: (state: { user: { role: string } }) => unknown) => selector({ user: { role: "admin" } }) }));
vi.mock("@/services/hotkey/hotkey-server/reports", () => ({ getReports: mocks.getReports, postReports: mocks.postReports, postReportsIdPreview: mocks.preview, postReportsIdBuild: mocks.build, postReportsIdPublish: mocks.publish }));
vi.mock("@/services/hotkey/hotkey-server/delivery", () => ({ getReportSubscriptions: mocks.getSubscriptions, postReportSubscriptions: vi.fn(), patchReportSubscriptionsId: vi.fn(), deleteReportSubscriptionsId: vi.fn(), postReportSubscriptionsIdRssTokenRotate: vi.fn() }));
vi.mock("@/services/hotkey/hotkey-server/knowledge", () => ({ getKnowledgeDocuments: mocks.getDocuments, getKnowledgeProposals: mocks.getProposals, postKnowledgeProposalsIdApprove: mocks.approve, postKnowledgeProposalsIdReject: mocks.reject, postKnowledgeProposalsIdApply: mocks.apply, postKnowledgeReconcile: mocks.reconcile }));

const report = {
  id: 7, version: 1, version_no: 1, type: "daily", timezone: "Asia/Shanghai", title: "每日热点报告", summary: "1 条事件快照", status: "draft",
  period_start: "2026-08-08T00:00:00+08:00", period_end: "2026-08-09T00:00:00+08:00",
  items: [{ event_id: 3, event_update_id: 9, rank: 1, title: "热点事件", summary: "可信摘要", heat_score: 88, evidence_set_hash: "a".repeat(64), reason_codes: ["rising"] }],
};

describe("ReportsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getReports.mockResolvedValue({ data: { items: [report] } });
    mocks.preview.mockResolvedValue({ data: { report, publishable: true } });
    mocks.postReports.mockResolvedValue({ data: report });
    mocks.getSubscriptions.mockResolvedValue({ data: { items: [] } });
    mocks.getDocuments.mockResolvedValue({ data: [{ id: 4, type: "report", reportID: 7, revisionNo: 0, status: "planned", vaultPath: "reports/daily-7-v1.md" }] });
    mocks.getProposals.mockResolvedValue({ data: [{ id: 5, version: 1, documentID: 4, baseRevisionNo: 0, status: "pending", diffSummary: "归档已发布报告快照", reason: "report_published", proposedBody: "# 每日热点报告" }] });
    mocks.approve.mockResolvedValue({ data: {} });
    mocks.reconcile.mockResolvedValue({ data: { scanned: 1, conflict: 0 } });
  });

  it("shows traceable report evidence in the read-only preview", async () => {
    const user = userEvent.setup();
    render(<ReportsPage />);
    expect(await screen.findByText("每日热点报告")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "预览" }));
    expect(await screen.findByText("#9")).toBeInTheDocument();
    expect(screen.getByText("a".repeat(64))).toBeInTheDocument();
    expect(mocks.preview).toHaveBeenCalledWith({ id: 7 });
  });

  it("creates the current period draft from the official dialog controls", async () => {
    const user = userEvent.setup();
    render(<ReportsPage />);
    await user.click(await screen.findByRole("button", { name: "新建报告" }));
    await user.click(screen.getByRole("button", { name: "生成草稿" }));
    await waitFor(() => expect(mocks.postReports).toHaveBeenCalledWith({ type: "daily", timezone: "Asia/Shanghai" }));
  });

  it("exposes approval and reconciliation only in the admin knowledge tab", async () => {
    const user = userEvent.setup();
    render(<ReportsPage />);
    expect(screen.getByRole("tab", { name: "知识归档" })).toBeInTheDocument();
    render(<KnowledgeArchive />);
    expect(await screen.findByText(/归档已发布报告快照/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "批准" }));
    await waitFor(() => expect(mocks.approve).toHaveBeenCalledWith({ id: 5, version: 1 }));
  });
});
