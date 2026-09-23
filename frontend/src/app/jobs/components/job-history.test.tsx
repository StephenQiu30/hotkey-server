import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { JobHistoryCard, JobHistoryContent } from "./job-history";

const job: HotKeyAPI.JobHistoryItemView = {
  id: "job-1",
  kind: "monitor.collect",
  source_key: "x",
  source_capability: "search",
  status: "partially_succeeded",
  requests_sent: 2,
  items_saved: 3,
  created_at: "2026-09-23T08:00:00Z",
  started_at: "2026-09-23T08:00:01Z",
  completed_at: "2026-09-23T08:00:03Z",
  next_run_at: null,
};

describe("job history card", () => {
  it("shows a localized status, persisted progress, and a detail link", () => {
    const html = renderToStaticMarkup(createElement(JobHistoryCard, { job }));

    expect(html).toContain("部分完成");
    expect(html).toContain("请求 2 次 · 已保存 3 条");
    expect(html).toContain('href="/jobs/job-1"');
    expect(html).not.toContain("scope");
    expect(html).not.toContain("operation_id");
  });
});

describe("job history content", () => {
  it("explains an empty history without implying a zero-result collection", () => {
    const html = renderToStaticMarkup(
      createElement(JobHistoryContent, {
        items: [],
        nextCursor: null,
        isLoadingMore: false,
        loadMoreError: null,
        onLoadMore: () => {},
      }),
    );

    expect(html).toContain("尚无任务记录");
    expect(html).toContain("当前账号还没有任务记录");
    expect(html).not.toContain("加载更多");
  });

  it("shows detail navigation and pagination when another page exists", () => {
    const html = renderToStaticMarkup(
      createElement(JobHistoryContent, {
        items: [job],
        nextCursor: "job-1",
        isLoadingMore: false,
        loadMoreError: null,
        onLoadMore: () => {},
      }),
    );

    expect(html).toContain('href="/jobs/job-1"');
    expect(html).toContain("加载更多");
  });

  it("keeps load-more failures visible without discarding the current page", () => {
    const html = renderToStaticMarkup(
      createElement(JobHistoryContent, {
        items: [job],
        nextCursor: "job-1",
        isLoadingMore: false,
        loadMoreError: "后续任务加载失败，请重试。",
        onLoadMore: () => {},
      }),
    );

    expect(html).toContain('role="alert"');
    expect(html).toContain("后续任务加载失败，请重试。");
    expect(html).toContain('href="/jobs/job-1"');
  });
});
