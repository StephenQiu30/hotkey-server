import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { APIErrorCode, UserRole } from "@/lib/domainEnums";
import { HotKeyAPIError } from "@/lib/request";

const mocks = vi.hoisted(() => ({
  id: "17",
  role: "editor",
  getMicroEvents: vi.fn(),
  getMicroEventsId: vi.fn(),
  getMicroEventsIdEvidence: vi.fn(),
  postMicroEventsIdFeedback: vi.fn(),
  postMicroEventsIdEvidenceEvidenceIdFeedback: vi.fn(),
}));

vi.mock("next/navigation", () => ({ useParams: () => ({ id: mocks.id }) }));
vi.mock("@/stores/authStore", () => ({
  useAuthStore: (selector: (state: { user: { role: string } }) => unknown) =>
    selector({ user: { role: mocks.role } }),
}));
vi.mock("@/services/hotkey/hotkey-server/microEvents", () => ({
  getMicroEvents: mocks.getMicroEvents,
  getMicroEventsId: mocks.getMicroEventsId,
  getMicroEventsIdEvidence: mocks.getMicroEventsIdEvidence,
  postMicroEventsIdFeedback: mocks.postMicroEventsIdFeedback,
  postMicroEventsIdEvidenceEvidenceIdFeedback: mocks.postMicroEventsIdEvidenceEvidenceIdFeedback,
}));

import EventGovernancePage from "@/app/dashboard/events/[id]/governance/page";

const detail: HotKeyAPI.MicroEventResponseDTO = {
  id: 17,
  version: 3,
  event_key: "event-openai-release",
  status: "active",
  primary_subject_key: "OpenAI",
  primary_action_key: "发布新模型",
  content_family_count: 1,
  document_count: 2,
  members: [{
    id: 31,
    version: 2,
    content_family_id: 41,
    membership_decision_id: 51,
    clustering_profile_version: "micro-event-clustering-v1",
  }],
};

const evidence: HotKeyAPI.ClaimEvidenceResponseDTO = {
  id: 61,
  version: 1,
  claim_id: 71,
  claim_version: 2,
  document_version_id: 81,
  text_quote_selector_id: 91,
  content_family_id: 41,
  subject: "OpenAI",
  predicate: "发布",
  object: "新模型",
  relation: "asserts",
  availability: "ready",
};

function resolveNormalState(overrides: Partial<HotKeyAPI.MicroEventResponseDTO> = {}) {
  mocks.getMicroEventsId.mockResolvedValue({ data: { ...detail, ...overrides } });
  mocks.getMicroEventsIdEvidence.mockResolvedValue({ data: { items: [evidence] } });
  mocks.getMicroEvents.mockResolvedValue({ data: { items: [{
    id: 18,
    version: 4,
    status: "active",
    primary_subject_key: "Anthropic",
    primary_action_key: "发布模型",
  }] } });
}

describe("EventGovernancePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.id = "17";
    mocks.role = UserRole.Editor;
    resolveNormalState();
  });

  it("renders the normal governance projection from generated clients", async () => {
    render(<EventGovernancePage />);

    expect(await screen.findByRole("heading", { name: "OpenAI · 发布新模型" })).toBeInTheDocument();
    expect(screen.getByText("#41")).toBeInTheDocument();
    expect(screen.getByText("#51")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "纠正 Evidence #61" })).toBeEnabled();
    expect(mocks.getMicroEventsId).toHaveBeenCalledWith({ id: 17 });
    expect(mocks.getMicroEventsIdEvidence).toHaveBeenCalledWith({ id: 17, limit: 50 });
    expect(mocks.getMicroEvents).toHaveBeenCalledWith({
      limit: 100,
      sort: "latest",
      status: "active,review_pending",
    });
  });

  it("keeps a dedicated loading state while durable facts are pending", () => {
    mocks.getMicroEventsId.mockReturnValue(new Promise(() => {}));
    mocks.getMicroEventsIdEvidence.mockReturnValue(new Promise(() => {}));
    mocks.getMicroEvents.mockReturnValue(new Promise(() => {}));

    render(<EventGovernancePage />);

    expect(screen.getByRole("status", { name: "正在加载事件治理" })).toBeInTheDocument();
  });

  it("distinguishes an event with no current governance facts", async () => {
    resolveNormalState({ members: [] });
    mocks.getMicroEventsIdEvidence.mockResolvedValue({ data: { items: [] } });

    render(<EventGovernancePage />);

    expect(await screen.findByText("暂无可治理成员或证据")).toBeInTheDocument();
  });

  it("shows a retryable load error and recovers without stale facts", async () => {
    mocks.getMicroEventsId
      .mockRejectedValueOnce(new Error("governance unavailable"))
      .mockResolvedValueOnce({ data: detail });

    render(<EventGovernancePage />);

    expect(await screen.findByText("事件治理加载失败")).toBeInTheDocument();
    expect(screen.getByText("governance unavailable")).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "重试" }));
    expect(await screen.findByRole("heading", { name: "OpenAI · 发布新模型" })).toBeInTheDocument();
  });

  it.each([
    [UserRole.Viewer, "当前账号为只读角色"],
    [UserRole.Analyst, "Analyst 在事件治理中为只读角色"],
  ])("renders the real %s permission boundary without mutation access", async (role, title) => {
    mocks.role = role;

    render(<EventGovernancePage />);

    expect(await screen.findByRole("alert", { name: "只读权限" })).toHaveTextContent(title);
    expect(screen.getByRole("button", { name: "归档事件" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "移出" })).toBeDisabled();
    expect(mocks.postMicroEventsIdFeedback).not.toHaveBeenCalled();
  });

  it.each([UserRole.Editor, UserRole.Admin])(
    "grants the real %s role event governance controls",
    async (role) => {
      mocks.role = role;

      render(<EventGovernancePage />);

      expect(await screen.findByRole("button", { name: "归档事件" })).toBeEnabled();
      expect(screen.getByRole("button", { name: "移出" })).toBeEnabled();
      expect(screen.getByRole("button", { name: "纠正 Evidence #61" })).toBeEnabled();
      expect(screen.queryByRole("alert", { name: "只读权限" })).not.toBeInTheDocument();
    },
  );

  it("keeps a server forbidden response as an explicit page state", async () => {
    mocks.getMicroEventsId.mockRejectedValue(
      new HotKeyAPIError(403, "forbidden", null, APIErrorCode.Forbidden),
    );

    render(<EventGovernancePage />);

    expect(await screen.findByRole("alert", { name: "权限不足" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "重试" })).not.toBeInTheDocument();
  });

  it("uses exact-version headers and stops on a concurrent update", async () => {
    mocks.postMicroEventsIdFeedback.mockRejectedValue(
      new HotKeyAPIError(409, "conflict", null, APIErrorCode.VersionConflict),
    );
    render(<EventGovernancePage />);
    await screen.findByRole("heading", { name: "OpenAI · 发布新模型" });

    await userEvent.setup().click(screen.getByRole("button", { name: "归档事件" }));

    expect(await screen.findByRole("alert", { name: "并发冲突" })).toHaveTextContent("事件版本冲突");
    expect(mocks.postMicroEventsIdFeedback).toHaveBeenCalledWith(
      { id: 17 },
      expect.objectContaining({
        action: "close_event",
        expected_event_version: 3,
        reason_code: "editor_reviewed",
      }),
      expect.objectContaining({
        headers: expect.objectContaining({
          "If-Match": '"v3"',
          "Idempotency-Key": expect.any(String),
        }),
      }),
    );
    expect(screen.getByRole("button", { name: "刷新最新版本" })).toBeInTheDocument();
  });

  it("submits Evidence correction through the generated exact-version client", async () => {
    mocks.postMicroEventsIdEvidenceEvidenceIdFeedback.mockResolvedValue({ data: { feedback_id: 101 } });
    render(<EventGovernancePage />);
    await screen.findByRole("heading", { name: "OpenAI · 发布新模型" });
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "纠正 Evidence #61" }));
    await user.type(screen.getByLabelText("新引用定位器编号"), "92");
    await user.click(screen.getByRole("button", { name: "追加纠正" }));

    await waitFor(() => expect(mocks.postMicroEventsIdEvidenceEvidenceIdFeedback).toHaveBeenCalledWith(
      { id: 17, evidence_id: 61 },
      expect.objectContaining({
        expected_claim_version: 2,
        result_text_quote_selector_id: 92,
        result_relation: "asserts",
        reason_code: "editor_reviewed",
      }),
      expect.objectContaining({ headers: expect.objectContaining({ "If-Match": '"v2"' }) }),
    ));
  });
});
