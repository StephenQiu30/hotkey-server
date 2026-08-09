import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { HotKeyAPIError } from "@/lib/request";

const mocks = vi.hoisted(() => ({
  getDraft: vi.fn(),
  putDraft: vi.fn(),
  submitExpansion: vi.fn(),
  getExpansion: vi.fn(),
  reviewCandidate: vi.fn(),
  submitPreview: vi.fn(),
  getPreview: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/monitorIntent", () => ({
  getMonitorsIdDraft: mocks.getDraft,
  putMonitorsIdDraftIntent: mocks.putDraft,
  postMonitorsIdDraftExpansionRuns: mocks.submitExpansion,
  getMonitorsIdDraftExpansionRunsRunId: mocks.getExpansion,
  postMonitorsIdDraftExpansionCandidatesCandidateIdDecision: mocks.reviewCandidate,
  postMonitorsIdDraftPreviewRuns: mocks.submitPreview,
  getMonitorsIdDraftPreviewRunsRunId: mocks.getPreview,
}));

import { MonitorIntentWorkspace } from "@/components/dashboard/MonitorIntentWorkspace";

const draft = {
  monitor_id: 7,
  draft_id: 12,
  resource_version: 3,
  objective: "跟踪 OpenAI 的正式产品发布",
  clauses: [{ operator: "must", field: "action", value: "发布" }],
  entities: [],
  examples: [{ label: "positive", text: "OpenAI 发布新模型" }],
  candidates: [],
} satisfies HotKeyAPI.IntentDraftResponseDTO;

describe("MonitorIntentWorkspace", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getDraft.mockResolvedValue({ data: draft });
    mocks.putDraft.mockResolvedValue({ data: { ...draft, resource_version: 4 } });
  });

  it("initializes an absent semantic intent with an explicit create precondition", async () => {
    mocks.getDraft.mockRejectedValueOnce(new HotKeyAPIError(404, "not found"));
    const user = userEvent.setup();
    render(<MonitorIntentWorkspace canAdmin monitorID={7} />);

    await user.type(await screen.findByLabelText("监控目标"), "监控 AI Agent 产品发布");
    await user.click(screen.getByRole("button", { name: "保存语义意图" }));

    await waitFor(() =>
      expect(mocks.putDraft).toHaveBeenCalledWith(
        { id: 7 },
        expect.objectContaining({
          expected_resource_version: 0,
          objective: "监控 AI Agent 产品发布",
        }),
        expect.objectContaining({
          headers: expect.objectContaining({ "If-None-Match": "*" }),
        }),
      ),
    );
  });

  it("replaces an existing draft with a strong resource ETag", async () => {
    const user = userEvent.setup();
    render(<MonitorIntentWorkspace canAdmin={false} monitorID={7} />);

    const objective = await screen.findByLabelText("监控目标");
    await user.clear(objective);
    await user.type(objective, "只跟踪正式模型发布");
    await user.click(screen.getByRole("button", { name: "保存语义意图" }));

    await waitFor(() =>
      expect(mocks.putDraft).toHaveBeenCalledWith(
        { id: 7 },
        expect.objectContaining({ expected_resource_version: 3 }),
        expect.objectContaining({
          headers: expect.objectContaining({ "If-Match": '"v3"' }),
        }),
      ),
    );
  });

  it("shows real expansion provenance and requires an admin decision", async () => {
    mocks.submitExpansion.mockResolvedValue({ data: { run_id: 31, status: "queued" } });
    mocks.getExpansion.mockResolvedValue({
      data: {
        run_id: 31,
        status: "succeeded",
        candidates: [
          {
            id: "candidate-1",
            value: "agentic workflow",
            source: "ai_model",
            reason: "目标中的 Agent 产品同义表达",
            model_version: "model-v2",
            prompt_version: "intent-expansion-v1",
            input_hash: "a".repeat(64),
            similarity: 0.82,
            risk: "medium",
            approval_status: "pending",
          },
        ],
      },
    });
    mocks.reviewCandidate.mockResolvedValue({
      data: {
        ...draft,
        resource_version: 4,
        candidates: [{ id: "candidate-1", value: "agentic workflow", approval_status: "approved" }],
      },
    });
    const user = userEvent.setup();
    render(<MonitorIntentWorkspace canAdmin monitorID={7} pollIntervalMs={10} />);

    await screen.findByDisplayValue("跟踪 OpenAI 的正式产品发布");
    await user.click(screen.getByRole("button", { name: "生成扩展候选" }));

    expect(mocks.submitExpansion).toHaveBeenCalledWith(
      { id: 7 },
      {
        expected_resource_version: 3,
        expansion_profile: "monitor-intent-expansion-v1",
      },
      expect.objectContaining({
        headers: expect.objectContaining({
          "If-Match": '"v3"',
          "Idempotency-Key": expect.any(String),
        }),
      }),
    );

    expect(await screen.findByText("agentic workflow")).toBeInTheDocument();
    expect(screen.getByText("目标中的 Agent 产品同义表达")).toBeInTheDocument();
    expect(screen.getByText(/model-v2/)).toBeInTheDocument();
    expect(screen.getByText(/intent-expansion-v1/)).toBeInTheDocument();
    expect(screen.queryByText(/可信|AI 已证实|82%/)).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "批准候选 agentic workflow" }));
    await waitFor(() =>
      expect(mocks.reviewCandidate).toHaveBeenCalledWith(
        { id: 7, candidate_id: "candidate-1" },
        expect.objectContaining({ expected_resource_version: 3, decision: "approved" }),
        expect.objectContaining({
          headers: expect.objectContaining({
            "If-Match": '"v3"',
            "Idempotency-Key": expect.any(String),
          }),
        }),
      ),
    );
  });

  it("renders preview channel ranks as raw signals, not percentages", async () => {
    mocks.submitPreview.mockResolvedValue({ data: { run_id: 41, status: "queued" } });
    mocks.getPreview.mockResolvedValue({
      data: {
        run_id: 41,
        status: "succeeded",
        preview: {
          estimated_alert_count: 1,
          warnings: ["semantic_recall_unavailable"],
          samples: [
            {
              document_version_id: 55,
              title: "OpenAI 发布新模型",
              decision: "candidate",
              reasons: ["must_action_matched"],
              exclusion_reasons: [],
              recall_signals: [{ channel: "lexical", rank: 1, raw_score: 0.74 }],
            },
          ],
        },
      },
    });
    const user = userEvent.setup();
    render(<MonitorIntentWorkspace canAdmin={false} monitorID={7} pollIntervalMs={10} />);

    await screen.findByDisplayValue("跟踪 OpenAI 的正式产品发布");
    await user.click(screen.getByRole("button", { name: "运行历史样本预览" }));

    expect(await screen.findByText("OpenAI 发布新模型")).toBeInTheDocument();
    expect(screen.getByText("lexical · 排名 1 · 原始信号 0.7400")).toBeInTheDocument();
    expect(screen.getByText("semantic_recall_unavailable")).toBeInTheDocument();
    expect(screen.queryByText("74%")).not.toBeInTheDocument();
  });
});
