import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { JobHealthSummaryView } from "./job-health-summary";

const issue: HotKeyAPI.JobContinuousFailureIssueView = {
  source_key: "x",
  source_capability: "search",
  configuration_ref: "monitor-config-1",
  configuration_version: 3,
  latest_failed_job_id: "job-3",
  failure: {
    error_code: "source.timeout",
    category: "transient",
    occurred_at: "2026-09-23T08:00:00Z",
    next_action: "检查来源连接",
    manual_retry_allowed: true,
  },
  consecutive_failure_threshold: 3,
};

describe("job health summary", () => {
  it("shows the repeated failure, safe next action, and latest task link", () => {
    const html = renderToStaticMarkup(
      createElement(JobHealthSummaryView, {
        state: { status: "ready", issues: [issue] },
      }),
    );

    expect(html).toContain("最近三条已结束任务均失败");
    expect(html).toContain("检查来源连接");
    expect(html).toContain("source.timeout");
    expect(html).toContain('href="/jobs/job-3"');
    expect(html).not.toContain("scope");
    expect(html).not.toContain("operation_id");
  });

  it("distinguishes no active issue from an unavailable issue summary", () => {
    const empty = renderToStaticMarkup(
      createElement(JobHealthSummaryView, {
        state: { status: "ready", issues: [] },
      }),
    );
    const failed = renderToStaticMarkup(
      createElement(JobHealthSummaryView, {
        state: { status: "error", message: "请求编号：request-1" },
      }),
    );

    expect(empty).toContain("当前没有连续失败提示");
    expect(empty).toContain('role="status"');
    expect(failed).toContain("任务历史仍可查看");
    expect(failed).toContain('role="alert"');
  });
});
