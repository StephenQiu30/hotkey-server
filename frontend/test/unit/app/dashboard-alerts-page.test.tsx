import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import AlertsPage from "@/app/dashboard/alerts/page";

const mocks = vi.hoisted(() => ({
  getAlerts: vi.fn(),
  getAlertsId: vi.fn(),
  postAlertsIdAcknowledge: vi.fn(),
  postAlertsIdSuppress: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/alerts", () => ({
  getAlerts: mocks.getAlerts,
  getAlertsId: mocks.getAlertsId,
  postAlertsIdAcknowledge: mocks.postAlertsIdAcknowledge,
  postAlertsIdResolve: vi.fn(),
  postAlertsIdSuppress: mocks.postAlertsIdSuppress,
}));

vi.mock("@/stores/authStore", () => ({
  useAuthStore: (selector: (state: { user: { role: string } }) => unknown) =>
    selector({ user: { role: "admin" } }),
}));

describe("AlertsPage", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getAlerts.mockResolvedValue({
      data: {
        items: [
          {
            id: 5,
            title: "化工安全事件快速升温",
            reason: "来源覆盖和传播动量同时超过阈值",
            severity: "critical",
            state: "open",
            occurrence_count: 3,
            version: 2,
            event_id: 11,
            last_triggered_at: "2026-08-04T14:10:00+08:00",
          },
        ],
      },
    });
    mocks.postAlertsIdAcknowledge.mockResolvedValue({ data: {} });
    mocks.postAlertsIdSuppress.mockResolvedValue({ data: {} });
    mocks.getAlertsId.mockResolvedValue({
      data: {
        thread: {
          id: 5,
          title: "化工安全事件快速升温",
          reason: "来源覆盖和传播动量同时超过阈值",
          severity: "critical",
          threshold: 70,
          min_heat: 70,
          min_momentum: 55,
          min_breadth: 25,
        },
        occurrences: [
          {
            id: 9,
            severity: "critical",
            final_score: 94,
            heat_score: 91,
            momentum_score: 88,
            breadth_score: 75,
            triggered_at: "2026-08-04T14:10:00+08:00",
          },
        ],
        email_deliveries: [
          {
            id: 3,
            severity: "critical",
            status: "retrying",
            attempt_count: 2,
            last_error: "temporary smtp failure",
          },
        ],
        audits: [],
      },
    });
  });

  it("loads explainable scores and safe email diagnostics in the detail dialog", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />);

    await user.click(
      await screen.findByRole("button", { name: "化工安全事件快速升温" })
    );

    expect(
      await screen.findByRole("heading", { name: "触发依据" })
    ).toBeInTheDocument();
    expect(
      screen.getByText("综合 94 · 热度 91 · 动量 88 · 宽度 75")
    ).toBeInTheDocument();
    expect(screen.getByText("等待重试")).toBeInTheDocument();
    expect(screen.getByText(/temporary smtp failure/)).toBeInTheDocument();
    expect(mocks.getAlertsId).toHaveBeenCalledWith({ id: 5 });
  });

  it("turns alert threads into an actionable inbox", async () => {
    render(<AlertsPage />);

    expect(
      await screen.findByRole("heading", { name: "告警中心" })
    ).toBeInTheDocument();
    expect(screen.getByText("化工安全事件快速升温")).toBeInTheDocument();
    expect(
      screen.getByRole("table", { name: "告警线程列表" })
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "确认告警" }));

    expect(mocks.postAlertsIdAcknowledge).toHaveBeenCalledWith(
      { id: 5 },
      { expected_version: 2, reason_code: "user_acknowledged" }
    );
  });

  it("requires confirmation before suppressing similar alerts", async () => {
    const user = userEvent.setup();
    render(<AlertsPage />);

    expect(await screen.findByText("化工安全事件快速升温")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "打开告警操作" }));
    await user.click(
      await screen.findByRole("menuitem", { name: "抑制同类告警" })
    );

    expect(
      await screen.findByRole("alertdialog", { name: "抑制同类告警？" })
    ).toBeInTheDocument();
    expect(mocks.postAlertsIdSuppress).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "确认抑制" }));

    expect(mocks.postAlertsIdSuppress).toHaveBeenCalledWith(
      { id: 5 },
      { expected_version: 2, reason_code: "user_suppressed" }
    );
  });
});
