import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ getMicroEvents: vi.fn() }));

vi.mock("@/services/hotkey/hotkey-server/microEvents", () => ({
  getMicroEvents: mocks.getMicroEvents,
}));

import EventsPage from "@/app/dashboard/events/page";

describe("EventsPage", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getMicroEvents.mockResolvedValue({
      data: {
        items: [
          {
            id: 17,
            version: 3,
            event_key: "event-openai-release",
            status: "active",
            primary_subject_key: "OpenAI",
            primary_action_key: "发布新模型",
            event_started_at: "2026-08-27T04:00:00Z",
            content_family_count: 2,
            document_count: 5,
            latest_heat: { heat_score: 82.45, reason_codes: ["velocity_rising", "multi_source"] },
            evidence_state: { state: "multiple_independent_origins" },
          },
        ],
      },
    });
  });

  it("renders accepted-match event projection with family-level counts", async () => {
    render(<EventsPage />);

    expect(await screen.findByText("OpenAI · 发布新模型")).toBeInTheDocument();
    expect(screen.getByText("2 个内容家族 · 5 篇文档")).toBeInTheDocument();
    expect(screen.getByText("82.5")).toBeInTheDocument();
    expect(screen.getByText("多个独立出处")).toBeInTheDocument();
    expect(mocks.getMicroEvents).toHaveBeenCalledWith({ limit: 50, sort: "heat" });
  });

  it("distinguishes an empty event projection", async () => {
    mocks.getMicroEvents.mockResolvedValueOnce({ data: { items: [] } });
    render(<EventsPage />);

    expect(await screen.findByText("暂时没有语义事件")).toBeInTheDocument();
  });

  it("shows a retryable event error", async () => {
    mocks.getMicroEvents
      .mockRejectedValueOnce(new Error("event service unavailable"))
      .mockResolvedValueOnce({ data: { items: [] } });
    render(<EventsPage />);

    expect(await screen.findByText("事件加载失败")).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "重试" }));
    expect(await screen.findByText("暂时没有语义事件")).toBeInTheDocument();
  });
});
