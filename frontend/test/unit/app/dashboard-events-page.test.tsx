import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import EventsPage from "@/app/dashboard/events/page";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getMicroEvents: vi.fn(),
  getMicroEventsId: vi.fn(),
  getMicroEventsIdEvidence: vi.fn(),
  getMonitors: vi.fn(),
  postMicroEventsIdEvidence: vi.fn(),
  postMicroEventsIdFeedback: vi.fn(),
  postMicroEventsIdEvidenceEvidenceIdFeedback: vi.fn(),
  postContentLineageDecisionsIdFeedback: vi.fn(),
  routerReplace: vi.fn(),
  navigationQuery: "",
}));

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(mocks.navigationQuery),
  useRouter: () => ({ replace: mocks.routerReplace }),
}));
vi.mock("@/services/hotkey/hotkey-server/microEvents", () => ({
  getMicroEvents: mocks.getMicroEvents,
  getMicroEventsId: mocks.getMicroEventsId,
  getMicroEventsIdEvidence: mocks.getMicroEventsIdEvidence,
  postMicroEventsIdEvidence: mocks.postMicroEventsIdEvidence,
  postMicroEventsIdFeedback: mocks.postMicroEventsIdFeedback,
  postMicroEventsIdEvidenceEvidenceIdFeedback: mocks.postMicroEventsIdEvidenceEvidenceIdFeedback,
}));
vi.mock("@/services/hotkey/hotkey-server/contentLineage", () => ({
  postContentLineageDecisionsIdFeedback: mocks.postContentLineageDecisionsIdFeedback,
}));
vi.mock("@/services/hotkey/hotkey-server/monitors", () => ({
  getMonitors: mocks.getMonitors,
}));

const event = {
  id: 11,
  version: 3,
  event_key: "acme-announcement",
  status: "active",
  primary_subject_key: "Acme",
  primary_action_key: "发布新项目",
  event_started_at: "2026-08-10T08:00:00Z",
  clustering_profile_version: "micro-event-clustering-v1",
  content_family_count: 2,
  document_count: 3,
	relevance_score: 0.91,
  storyline: { id: 4, version: 1, title: "Acme 产品进展", summary: "多个具体发布事件的长期脉络", status: "active" },
  latest_heat: { id: 8, micro_event_version: 3, heat_score: 72.5, independent_lineage_root_count: 2 },
  evidence_state: { id: 9, event_version: 3, state: "conflicting_reports", independent_origin_count: 2, algorithm_version: "evidence-state-lineage-v2" },
  evidence_summary: {
    id: 10,
    event_version: 3,
    summary_profile_version: "evidence-summary-v1",
    sentences: [
      { id: 12, ordinal: 0, text: "Acme 发布了 Project Nova。", editorial_note: false, decision_origin: "automatic", claim_evidence_version_ids: [22] },
      { id: 13, ordinal: 1, text: "发布时间由编辑复核。", editorial_note: true, decision_origin: "manual", claim_evidence_version_ids: [23] },
    ],
  },
} satisfies HotKeyAPI.MicroEventResponseDTO;

const readyEvidence = {
  id: 21,
  version: 2,
  claim_id: 17,
  document_version_id: 31,
  text_quote_selector_id: 41,
  content_family_id: 51,
  lineage_root_document_version_id: 31,
  lineage_decision_id: 61,
  content_family_member_version: 4,
  subject: "Acme",
  predicate: "发布",
  object: "Project Nova",
  relation: "asserts",
  availability: "ready",
  exact_quote: "Acme 正式发布 Project Nova。",
  canonical_url: "https://publisher.example/articles/nova",
  source_record_url: "https://discussion.example/items/7",
  publisher: "Acme Newsroom",
  captured_at: "2026-08-10T08:03:00Z",
  published_at: "2026-08-10T08:00:00Z",
  extraction_schema_version: "atomic-claim-evidence-v2",
  decision_origin: "manual",
} satisfies HotKeyAPI.ClaimEvidenceResponseDTO;

describe("EventsPage v2", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.navigationQuery = "";
    useAuthStore.setState({ user: { role: "viewer" } as HotKeyAPI.UserResponse });
    mocks.getMicroEvents.mockResolvedValue({ data: { items: [event] } });
    mocks.getMicroEventsId.mockResolvedValue({ data: event });
    mocks.getMicroEventsIdEvidence.mockResolvedValue({ data: { items: [readyEvidence] } });
	mocks.getMonitors.mockResolvedValue({ data: { items: [{ id: 7, name: "AI 产品", status: "active" }] } });
    mocks.postMicroEventsIdEvidence.mockResolvedValue({ data: {} });
    mocks.postMicroEventsIdFeedback.mockResolvedValue({ data: {} });
    mocks.postMicroEventsIdEvidenceEvidenceIdFeedback.mockResolvedValue({ data: {} });
    mocks.postContentLineageDecisionsIdFeedback.mockResolvedValue({ data: {} });
  });

  it("loads semantic events, Storyline, Heat v2, and exact evidence in parallel", async () => {
    render(<EventsPage />);
    expect(await screen.findByRole("heading", { name: "Acme · 发布新项目" })).toBeInTheDocument();
    expect(await screen.findByText("Acme 正式发布 Project Nova。")).toBeInTheDocument();
    expect(screen.getAllByText("Acme 产品进展").length).toBeGreaterThan(0);
    expect(screen.getByText("72.5")).toBeInTheDocument();
    expect(screen.getByText("Acme 发布了 Project Nova。")).toBeInTheDocument();
    expect(screen.getByText("发布时间由编辑复核。")).toBeInTheDocument();
    expect(screen.getByText("人工编辑")).toBeInTheDocument();
    expect(screen.getByText("证据版本：#22")).toBeInTheDocument();
    expect(screen.getAllByText("存在相反表述").length).toBeGreaterThan(0);
		expect(screen.getByText("热度 72.5")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /阅读归档/ })).toHaveAttribute("href", expect.stringContaining("/dashboard/document-versions/31"));
    expect(mocks.getMicroEventsId).toHaveBeenCalledWith({ id: 11 });
    expect(mocks.getMicroEventsIdEvidence).toHaveBeenCalledWith({ id: 11, limit: 100 });
		expect(mocks.getMicroEvents).toHaveBeenCalledWith({ limit: 30, sort: "heat", cursor: undefined });
  });

	it("requests latest ordering from the server instead of sorting locally", async () => {
		render(<EventsPage />);
		await screen.findByRole("heading", { name: "Acme · 发布新项目" });
		await userEvent.click(screen.getByRole("combobox", { name: "排序" }));
		await userEvent.click(screen.getByRole("option", { name: "最新发现" }));
		await waitFor(() => expect(mocks.getMicroEvents).toHaveBeenLastCalledWith({ limit: 30, sort: "latest", cursor: undefined }));
	});

	it("combines monitor, source, evidence, and event-time filters with relevance ordering", async () => {
		render(<EventsPage />);
		await screen.findByRole("heading", { name: "Acme · 发布新项目" });

		await userEvent.click(screen.getByRole("combobox", { name: "排序" }));
		await userEvent.click(screen.getByRole("option", { name: "相关性最高" }));
		await userEvent.click(screen.getByRole("combobox", { name: "监控器" }));
		await userEvent.click(await screen.findByRole("option", { name: "AI 产品" }));
		await userEvent.click(screen.getByRole("combobox", { name: "来源" }));
		await userEvent.click(screen.getByRole("option", { name: "X / Twitter" }));
		await userEvent.click(screen.getByRole("combobox", { name: "证据状态" }));
		await userEvent.click(screen.getByRole("option", { name: "多个独立起源" }));
		fireEvent.change(screen.getByLabelText("事件开始时间从"), { target: { value: "2026-08-01T08:30" } });
		fireEvent.change(screen.getByLabelText("事件开始时间到"), { target: { value: "2026-08-12T18:00" } });

		await waitFor(() => expect(mocks.getMicroEvents).toHaveBeenLastCalledWith({
			limit: 30,
			sort: "relevance",
			cursor: undefined,
			monitor_id: 7,
			source_type: "x",
			evidence_state: "multiple_origins",
			started_from: new Date("2026-08-01T08:30").toISOString(),
			started_to: new Date("2026-08-12T18:00").toISOString(),
		}));
		expect(screen.getByText("相关性 91%")).toBeInTheDocument();
	});

  it("never renders legacy truth or credibility semantics", async () => {
    const { container } = render(<EventsPage />);
    await screen.findByText("Acme 正式发布 Project Nova。");
    expect(container.textContent).not.toMatch(/可信度|真实概率|多源证实|确认强度|数据置信度/);
  });

  it("does not expose quote text or hashes when rights are unavailable", async () => {
    mocks.getMicroEventsIdEvidence.mockResolvedValue({ data: { items: [{ ...readyEvidence, availability: "rights_unavailable", exact_quote: undefined, quote_sha256: undefined, plaintext_sha256: undefined }] } });
    render(<EventsPage />);
    expect(await screen.findByText("引用权利已失效")).toBeInTheDocument();
    expect(screen.queryByText("Acme 正式发布 Project Nova。")).not.toBeInTheDocument();
    expect(screen.getByText(/当前只展示引用状态/)).toBeInTheDocument();
  });

  it("keeps mutation controls hidden from viewers", async () => {
    render(<EventsPage />);
    await screen.findByText("Acme 正式发布 Project Nova。");
    expect(screen.queryByRole("heading", { name: "人工复核" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /纠正关系或摘录/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /复核正文谱系/ })).not.toBeInTheDocument();
  });

  it("shows versioned review controls to editors and deep-links selected events", async () => {
    useAuthStore.setState({ user: { role: "editor" } as HotKeyAPI.UserResponse });
    render(<EventsPage />);
    expect(await screen.findByRole("heading", { name: "人工复核" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /纠正关系或摘录/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /复核正文谱系/ })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Acme · 发布新项目/ }));
    expect(mocks.routerReplace).toHaveBeenCalledWith("/dashboard/events?event=11", { scroll: false });
  });
});
