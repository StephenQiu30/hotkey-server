import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { EventEvidenceTimeline } from "@/components/dashboard/EventEvidenceTimeline";

describe("EventEvidenceTimeline", () => {
  it("shows truthful n/m evidence counts, partial failures and both reading paths", () => {
    render(
      <EventEvidenceTimeline
        contents={[
          {
            id: 11,
            title: "An archived research item",
            source_name: "arXiv · AI",
            canonical_url: "https://example.test/items/11",
          },
          {
            id: 12,
            title: "A second research item",
            source_name: "Science",
          },
        ]}
        failedCount={1}
        totalCount={3}
      />,
    );

    expect(screen.getByRole("heading", { name: "证据验证" })).toBeInTheDocument();
    expect(screen.getByText("已读取 2 条，1 条暂不可读")).toBeInTheDocument();
    expect(screen.getAllByText("可读")).toHaveLength(2);
    expect(screen.getAllByText("可读")[0]).toHaveClass("inline-flex");
    expect(screen.getByRole("link", { name: "全部证据" })).toHaveClass("inline-flex");
    expect(
      screen.getByRole("link", { name: "阅读归档：An archived research item" }),
    ).toHaveAttribute("href", "/dashboard/contents/11");
    expect(
      screen.getByRole("link", { name: "访问原站：An archived research item" }),
    ).toHaveAttribute("href", "https://example.test/items/11");
    expect(
      screen.getByRole("link", { name: "阅读归档：A second research item" }),
    ).toHaveAttribute("href", "/dashboard/contents/12");
    expect(
      screen.queryByRole("link", { name: "访问原站：A second research item" }),
    ).not.toBeInTheDocument();
  });

  it("preserves total membership when no detail can be read", () => {
    render(
      <EventEvidenceTimeline contents={[]} failedCount={2} totalCount={2} />,
    );

    expect(screen.getByText("已读取 0 条，2 条暂不可读")).toBeInTheDocument();
    expect(screen.getByText("该事件有证据成员，但详情暂不可读。")).toBeInTheDocument();
  });
});
