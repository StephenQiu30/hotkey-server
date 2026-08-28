import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ReportsPage from "@/app/dashboard/reports/page";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { HotKeyAPIError } from "@/lib/request";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getReports: vi.fn(),
  postReports: vi.fn(),
  postReportsIdApprove: vi.fn(),
  postReportsIdBuild: vi.fn(),
  postReportsIdReject: vi.fn(),
  postReportsIdSubmit: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/reports", () => mocks);

const pendingReport = {
  id: 41,
  version: 2,
  version_no: 1,
  type: "daily",
  title: "芯片行业日报",
  summary: "可信摘要",
  body: '<svg onload="sentinel">只是文本',
  status: "pending_approval",
  period_start: "2026-08-28T00:00:00Z",
  period_end: "2026-08-29T00:00:00Z",
  timezone: "Asia/Shanghai",
  items: [{
    micro_event_id: 9,
    rank: 1,
    title: "新品发布",
    summary: "发布摘要",
    heat_score: 88,
    sentences: [{ source_summary_sentence_id: 11, ordinal: 1, text: "事实句", claim_evidence_version_ids: [71] }],
  }],
} satisfies HotKeyAPI.ReportResponse;

function setRole(role: UserRole) {
  useAuthStore.setState({ status: AuthStatus.Authenticated, user: { id: 1, email: "actor@example.test", role }, error: null });
}

describe("ReportsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.history.replaceState({}, "", "/dashboard/reports?report=41");
    setRole(UserRole.Editor);
    mocks.getReports.mockResolvedValue({ data: { items: [pendingReport] } });
    mocks.postReportsIdApprove.mockResolvedValue({ data: { ...pendingReport, version: 3, status: "published", frozen: true } });
  });

  it("opens a notification deep link and approves the exact pending revision", async () => {
    const user = userEvent.setup();
    render(<ReportsPage />);

    expect(await screen.findByRole("heading", { name: "芯片行业日报" })).toBeInTheDocument();
    expect(screen.getByText("Evidence IDs：71")).toBeInTheDocument();
    expect(screen.getByText(/<svg onload="sentinel">/)).toBeInTheDocument();
    expect(document.querySelector("svg[onload]")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "批准并冻结" }));
    await waitFor(() => expect(mocks.postReportsIdApprove).toHaveBeenCalledWith({ id: 41 }, { expected_resource_version: 2 }));
    expect(await screen.findByText("已冻结")).toBeInTheDocument();
  });

  it("keeps analysts inside the documented action boundary", async () => {
    setRole(UserRole.Analyst);
    mocks.getReports.mockResolvedValue({ data: { items: [{ ...pendingReport, status: "draft", version: 1 }] } });
    render(<ReportsPage />);
    expect(await screen.findByRole("button", { name: "提交审批" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "批准并冻结" })).not.toBeInTheDocument();
  });

  it("renders loading, empty and direct permission failures distinctly", async () => {
    let resolveReports!: (value: unknown) => void;
    mocks.getReports.mockReturnValueOnce(new Promise((resolve) => { resolveReports = resolve; }));
    const { unmount } = render(<ReportsPage />);
    expect(await screen.findByLabelText("正在加载日报")).toBeInTheDocument();
    resolveReports({ data: { items: [] } });
    expect(await screen.findByRole("heading", { name: "尚无日报 Revision" })).toBeInTheDocument();
    unmount();

    mocks.getReports.mockRejectedValueOnce(new HotKeyAPIError(403, "forbidden"));
    render(<ReportsPage />);
    expect(await screen.findByLabelText("日报权限不足")).toBeInTheDocument();
  });
});
