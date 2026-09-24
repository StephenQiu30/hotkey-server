import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  JobCoverageWindows,
  JobResult,
  JobSourceFreshness,
} from "./job-detail";

describe("job result", () => {
  it("links a persisted result to the existing content detail page", () => {
    const html = renderToStaticMarkup(
      createElement(JobResult, {
        resultContentId: "content-1",
        savedDescription: "已持久保存 1 条结果。",
        updatedAt: "2026-09-23T00:00:00Z",
      }),
    );

    expect(html).toContain('href="/content/content-1"');
    expect(html).toContain("打开作品资料");
    expect(html).not.toContain("当前没有可打开的作品资料");
  });

  it("keeps an honest state when no result is available", () => {
    const html = renderToStaticMarkup(
      createElement(JobResult, {
        resultContentId: null,
        savedDescription: "尚未保存结果；这不等于来源返回空结果。",
        updatedAt: "2026-09-23T00:00:00Z",
      }),
    );

    expect(html).toContain("当前没有可打开的作品资料");
    expect(html).not.toContain("打开作品资料");
  });
});

describe("job source freshness", () => {
  it("separates recent attempts from full success and labels budget delay", () => {
    const html = renderToStaticMarkup(
      createElement(JobSourceFreshness, {
        freshness: {
          last_attempt_at: "2026-09-24T11:55:00Z",
          last_success_at: "2026-09-24T10:00:00Z",
          delay_reason: "budget_exhausted",
          delay_since_at: "2026-09-24T11:58:30Z",
          delay_duration_us: 90_000_000,
        },
      }),
    );

    expect(html).toContain("最近尝试");
    expect(html).toContain("最近完整成功");
    expect(html).toContain("采集预算耗尽");
    expect(html).toContain("1 分钟 30 秒");
  });

  it("keeps absent history explicit without inventing a current delay", () => {
    const html = renderToStaticMarkup(
      createElement(JobSourceFreshness, {
        freshness: {
          last_attempt_at: null,
          last_success_at: null,
          delay_reason: null,
          delay_since_at: null,
          delay_duration_us: null,
        },
      }),
    );

    expect(html).toContain("尚无执行尝试");
    expect(html).toContain("尚无完整成功记录");
    expect(html).not.toContain("当前延期");
  });
});

describe("job coverage windows", () => {
  it("shows only persisted ranges without implying coverage beyond them", () => {
    const html = renderToStaticMarkup(
      createElement(JobCoverageWindows, {
        windows: [
          {
            id: "window-1",
            starts_at: "2026-09-21T00:00:00Z",
            ends_at: "2026-09-22T00:00:00Z",
            status: "partial",
            stop_reason: "cursor_loop",
            page_count: 2,
          },
        ],
      }),
    );

    expect(html).toContain("采集窗口记录");
    expect(html).toContain("部分完成");
    expect(html).toContain("检测到重复分页");
    expect(html).toContain("2 页");
    expect(html).toContain("不代表未记录范围已完整覆盖");
  });

  it("does not show an empty window block", () => {
    const html = renderToStaticMarkup(
      createElement(JobCoverageWindows, { windows: [] }),
    );

    expect(html).toBe("");
  });
});
