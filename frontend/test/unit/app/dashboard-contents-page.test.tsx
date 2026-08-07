import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ContentsPage from "@/app/dashboard/contents/page";

const mocks = vi.hoisted(() => ({
  role: "editor",
  getCollectionRuns: vi.fn(),
  retryCollectionRun: vi.fn(),
  getContents: vi.fn(),
  deleteContentsId: vi.fn(),
  getSourceConnections: vi.fn(),
  getMonitors: vi.fn(),
  routerReplace: vi.fn(),
  navigationQuery: "",
}));

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(mocks.navigationQuery),
  useRouter: () => ({ replace: mocks.routerReplace }),
}));

vi.mock("@/services/hotkey/hotkey-server/collectionRuns", () => ({
  getCollectionRuns: mocks.getCollectionRuns,
  postCollectionRunsIdRetry: mocks.retryCollectionRun,
}));
vi.mock("@/services/hotkey/hotkey-server/contents", () => ({
  getContents: mocks.getContents,
  deleteContentsId: mocks.deleteContentsId,
}));
vi.mock("@/services/hotkey/hotkey-server/sources", () => ({
  getSourceConnections: mocks.getSourceConnections,
}));
vi.mock("@/services/hotkey/hotkey-server/monitors", () => ({
  getMonitors: mocks.getMonitors,
}));
vi.mock("@/stores/authStore", () => ({
  useAuthStore: (selector: (state: { user: { role: string } }) => unknown) =>
    selector({ user: { role: mocks.role } }),
}));

describe("ContentsPage pagination", () => {
  beforeEach(() => {
    mocks.role = "editor";
    mocks.navigationQuery = "";
    mocks.getSourceConnections.mockResolvedValue({ data: { items: [] } });
    mocks.getMonitors.mockResolvedValue({ data: { items: [] } });
  });

  it("restores URL filters and sends the complete query to the content API", async () => {
    mocks.navigationQuery =
      "q=%E5%8F%91%E5%B8%83&source=3&monitor=7&from=2026-08-01&to=2026-08-02&decision=accepted&sort=relevance&limit=50";
    mocks.getCollectionRuns.mockResolvedValue({ data: { items: [] } });
    mocks.getContents.mockResolvedValue({ data: { items: [] } });

    render(<ContentsPage />);

    await waitFor(() =>
      expect(mocks.getContents).toHaveBeenCalledWith({
        q: "发布",
        source_connection_id: 3,
        monitor_id: 7,
        published_from: "2026-08-01T00:00:00Z",
        published_to: "2026-08-02T23:59:59Z",
        decision: "accepted",
        sort: "relevance",
        limit: 50,
      })
    );
    expect(
      screen.getByRole("link", { name: "在事件中搜索同一关键词" })
    ).toHaveAttribute("href", "/dashboard/events?q=%E5%8F%91%E5%B8%83");
  });

  it("debounces content search, resets its cursor and persists the query", async () => {
    mocks.getCollectionRuns.mockResolvedValue({ data: { items: [] } });
    mocks.getContents.mockResolvedValue({ data: { items: [] } });
    render(<ContentsPage />);

    await userEvent
      .setup()
      .type(screen.getByRole("searchbox", { name: "搜索内容" }), "更新");

    await waitFor(() =>
      expect(mocks.getContents).toHaveBeenLastCalledWith({
        limit: 20,
        q: "更新",
      })
    );
    expect(mocks.getCollectionRuns).toHaveBeenCalledTimes(1);
    expect(mocks.routerReplace).toHaveBeenCalledWith(
      "/dashboard/contents?q=%E6%9B%B4%E6%96%B0",
      { scroll: false }
    );
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("loads readable contents without requesting collection runs for a viewer", async () => {
    mocks.role = "viewer";
    mocks.getContents.mockResolvedValue({
      data: { items: [{ id: 7, title: "Fetched content" }] },
    });

    render(<ContentsPage />);

    expect(await screen.findByText("Fetched content")).toBeInTheDocument();
    expect(mocks.getContents).toHaveBeenCalledWith({ limit: 20 });
    expect(mocks.getCollectionRuns).not.toHaveBeenCalled();
  });

  it("passes the collection cursor when navigating to the next page", async () => {
    mocks.getCollectionRuns
      .mockResolvedValueOnce({
        data: {
          items: [{ id: 1, status: "succeeded" }],
          next_cursor: "run-cursor-1",
        },
      })
      .mockResolvedValueOnce({ data: { items: [] } });
    mocks.getContents.mockResolvedValue({ data: { items: [] } });

    render(<ContentsPage />);
    const nextButtons = await screen.findAllByRole("button", {
      name: "下一页",
    });
    await userEvent.setup().click(nextButtons[0]);

    await waitFor(() =>
      expect(mocks.getCollectionRuns).toHaveBeenLastCalledWith({
        cursor: "run-cursor-1",
        limit: 20,
      })
    );
  });

  it("reloads both collection lists with the selected page size", async () => {
    mocks.getCollectionRuns.mockResolvedValue({
      data: { items: [{ id: 1, status: "succeeded" }] },
    });
    mocks.getContents.mockResolvedValue({
      data: { items: [{ id: 7, title: "Fetched content" }] },
    });

    render(<ContentsPage />);
    const pageSizeSelectors = await screen.findAllByRole("combobox", {
      name: "每页条数",
    });
    const user = userEvent.setup();
    await user.click(pageSizeSelectors[0]);
    await user.click(screen.getByRole("option", { name: "50 条" }));

    await waitFor(() => {
      expect(mocks.getCollectionRuns).toHaveBeenLastCalledWith({ limit: 50 });
      expect(mocks.getContents).toHaveBeenLastCalledWith({ limit: 50 });
    });
  });

  it("confirms and deletes a fetched content, then refreshes the content page", async () => {
    mocks.getCollectionRuns.mockResolvedValue({ data: { items: [] } });
    mocks.getContents.mockResolvedValue({
      data: { items: [{ id: 7, title: "Fetched content" }] },
    });
    mocks.deleteContentsId.mockResolvedValue({ data: null });

    render(<ContentsPage />);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "删除内容：Fetched content" })
    );
    expect(screen.getByText("删除采集内容")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "确认删除" }));

    await waitFor(() =>
      expect(mocks.deleteContentsId).toHaveBeenCalledWith({ id: 7 })
    );
    expect(mocks.getContents).toHaveBeenCalledTimes(2);
  });

  it("lets an administrator retry a failed collection run", async () => {
    mocks.role = "admin";
    mocks.getCollectionRuns
      .mockResolvedValueOnce({
        data: { items: [{ id: 3, status: "failed", error_code: "temporary" }] },
      })
      .mockResolvedValueOnce({
        data: { items: [{ id: 3, status: "queued" }] },
      });
    mocks.getContents.mockResolvedValue({ data: { items: [] } });
    mocks.retryCollectionRun.mockResolvedValue({
      data: { id: 3, status: "queued" },
    });

    render(<ContentsPage />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "重试采集批次 #3" }));

    await waitFor(() =>
      expect(mocks.retryCollectionRun).toHaveBeenCalledWith({ id: 3 })
    );
    expect(mocks.getCollectionRuns).toHaveBeenCalledTimes(2);
  });

  it("lets an editor inspect a failed run without exposing manual retry", async () => {
    mocks.role = "editor";
    mocks.getCollectionRuns.mockResolvedValue({
      data: { items: [{ id: 3, status: "failed", error_code: "temporary" }] },
    });
    mocks.getContents.mockResolvedValue({ data: { items: [] } });

    render(<ContentsPage />);

    expect(await screen.findByText("temporary")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "重试采集批次 #3" })
    ).not.toBeInTheDocument();
    expect(mocks.getCollectionRuns).toHaveBeenCalledWith({ limit: 20 });
  });
});
