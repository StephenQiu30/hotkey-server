import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  override: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/documentMatches", () => ({
  getMonitorsIdDocumentMatches: mocks.list,
  postMonitorsIdDocumentMatchesMatchDecisionIdOverrides: mocks.override,
}));

import { DocumentMatchWorkspace } from "@/components/dashboard/DocumentMatchWorkspace";

const reviewMatch = {
  match_decision_id: 31,
  monitor_id: 7,
  monitor_version_id: 11,
  compiled_profile_id: 13,
  document_version_id: 73,
  relevance_profile_id: 17,
  matching_algorithm_version: "hybrid-rrf-v1",
  reranker_version: "reranker-shadow-v1",
  calibration_version: "uncalibrated-v1",
  rrf_score: 0.0324,
  automatic_decision: "review",
  effective_decision: "review",
  degraded: true,
  reason_codes: ["semantic_recall_unavailable"],
  resource_version: 0,
  decided_at: "2026-08-10T02:00:00Z",
  signals: [
    { channel: "lexical", rank: 1, raw_score: 0.83, algorithm_version: "lexical-v1" },
  ],
} satisfies HotKeyAPI.DocumentMatchResponseDTO;

describe("DocumentMatchWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.list.mockResolvedValue({ data: { items: [reviewMatch], next_cursor: "" } });
    mocks.override.mockResolvedValue({
      data: {
        override_id: 91,
        match_decision_id: 31,
        monitor_id: 7,
        monitor_version_id: 11,
        document_version_id: 73,
        previous_effective_decision: "review",
        decision: "accepted",
        reason_code: "manual_relevant",
        note: "与监控目标直接相关",
        actor_user_id: 5,
        resource_version: 1,
        created_at: "2026-08-10T02:05:00Z",
        reused: false,
      },
    });
  });

  afterEach(cleanup);

  it("renders exact-version relevance facts without truth or credibility language", async () => {
    render(<DocumentMatchWorkspace canReview monitorID={7} />);

    expect(await screen.findByRole("link", { name: "正文版本 #73" })).toHaveAttribute(
      "href",
      "/dashboard/document-versions/73",
    );
    expect(screen.getAllByText("等待人工判断").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("相关概率尚未校准")).toBeInTheDocument();
    expect(screen.getByText(/lexical · 排名 1 · 原始信号 0.8300/)).toBeInTheDocument();
    expect(screen.queryByText(/可信|真实|已核实|多源印证/)).not.toBeInTheDocument();
    expect(mocks.list).toHaveBeenCalledWith({ id: 7, decision: "review", limit: 50 });
  });

  it("appends an exact manual override with optimistic concurrency headers", async () => {
    render(<DocumentMatchWorkspace canReview monitorID={7} />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "标记相关" }));
    await user.type(screen.getByLabelText("复核说明（可选）"), "与监控目标直接相关");
    await user.click(screen.getByRole("button", { name: "确认标记相关" }));

    await waitFor(() =>
      expect(mocks.override).toHaveBeenCalledWith(
        { id: 7, match_decision_id: 31 },
        {
          decision: "accepted",
          reason_code: "manual_relevant",
          note: "与监控目标直接相关",
        },
        {
          headers: expect.objectContaining({
            "If-Match": '"v0"',
            "Idempotency-Key": expect.stringMatching(/^document-match-review-7-31-/),
          }),
        },
      ),
    );
    expect((await screen.findAllByText("已标记相关")).length).toBeGreaterThanOrEqual(2);
  });

  it("keeps viewer access read-only and supports decision filtering", async () => {
    render(<DocumentMatchWorkspace canReview={false} monitorID={7} />);
    const user = userEvent.setup();

    expect(await screen.findByRole("link", { name: "正文版本 #73" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "标记相关" })).not.toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText("判定筛选"), "accepted");
    await waitFor(() =>
      expect(mocks.list).toHaveBeenLastCalledWith({ id: 7, decision: "accepted", limit: 50 }),
    );
  });
});
