import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SearchPage from "@/app/dashboard/search/page";
import { HotKeyAPIError } from "@/lib/request";

const mocks = vi.hoisted(() => ({ getSearch: vi.fn() }));

vi.mock("@/services/hotkey/hotkey-server/search", () => ({ getSearch: mocks.getSearch }));

describe("SearchPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getSearch.mockResolvedValue({
      data: {
        items: [{
          type: "content",
          id: 42,
          title: '<svg onload="sentinel"> Release',
          title_highlight: "&lt;svg onload=&#34;sentinel&#34;&gt; <mark>Release</mark>",
          snippet: "芯片发布摘要",
          snippet_highlight: "芯片发布摘要",
          status: "active",
          occurred_at: "2026-08-28T08:00:00Z",
          score: 0.8,
        }],
      },
    });
  });

  it("uses the persisted PostgreSQL read path and renders controlled highlights as React nodes", async () => {
    const user = userEvent.setup();
    render(<SearchPage />);
    await user.type(screen.getByLabelText("搜索词"), "Release");
    await user.click(screen.getByRole("button", { name: "搜索" }));
    await waitFor(() => expect(mocks.getSearch).toHaveBeenCalledWith({ q: "Release", limit: 50, sort: "relevance" }));
    expect(await screen.findByRole("heading", { name: /Release/ })).toBeInTheDocument();
    expect(document.querySelector("mark")).toHaveTextContent("Release");
    expect(document.querySelector("svg[onload]")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Release/ })).toHaveAttribute("href", "/dashboard/contents/42");
    expect(screen.getAllByText("内容")).toHaveLength(2);
  });

  it("supports bounded resource and object filters without invoking external instant search", async () => {
    const user = userEvent.setup();
    render(<SearchPage />);
    await user.type(screen.getByLabelText("搜索词"), "芯片");
    await user.click(screen.getByRole("checkbox", { name: "事件" }));
    await user.click(screen.getByRole("checkbox", { name: "知识" }));
    await user.click(screen.getByRole("button", { name: "高级筛选" }));
    await user.type(screen.getByLabelText("来源 ID"), "9");
    await user.type(screen.getByLabelText("Monitor ID"), "12");
    await user.type(screen.getByLabelText("实体"), "Acme-42");
    await user.click(screen.getByRole("button", { name: "搜索" }));
    await waitFor(() => expect(mocks.getSearch).toHaveBeenCalledWith({
      q: "芯片",
      limit: 50,
      sort: "relevance",
      types: "content",
      source_connection_id: 9,
      monitor_id: 12,
      entity: "Acme-42",
    }));
  });

  it("renders loading and empty states", async () => {
    const user = userEvent.setup();
    let resolveSearch!: (value: unknown) => void;
    mocks.getSearch.mockReturnValueOnce(new Promise((resolve) => { resolveSearch = resolve; }));
    render(<SearchPage />);
    await user.type(screen.getByLabelText("搜索词"), "missing");
    await user.keyboard("{Enter}");
    expect(await screen.findByLabelText("正在加载搜索结果")).toBeInTheDocument();
    resolveSearch({ data: { items: [] } });
    expect(await screen.findByRole("heading", { name: "没有符合条件的结果" })).toBeInTheDocument();
  });

  it("renders a distinct permission state", async () => {
    const user = userEvent.setup();
    mocks.getSearch.mockRejectedValueOnce(new HotKeyAPIError(403, "forbidden"));
    render(<SearchPage />);
    await user.type(screen.getByLabelText("搜索词"), "private");
    await user.keyboard("{Enter}");
    expect(await screen.findByText("没有检索权限")).toBeInTheDocument();
    expect(screen.queryByText("搜索失败")).not.toBeInTheDocument();
  });
});
