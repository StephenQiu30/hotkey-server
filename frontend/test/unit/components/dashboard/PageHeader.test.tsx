import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PageHeader } from "@/components/dashboard/PageHeader";

describe("PageHeader", () => {
  it("stacks the action below the title on compact screens", () => {
    render(
      <PageHeader
        eyebrow="Sources"
        title="来源管理"
        description="连接论文 RSS 数据源。"
        action={<button type="button">新增来源</button>}
      />,
    );

    const header = screen.getByRole("group", { name: "来源管理" });
    expect(header).toHaveClass("flex-col", "sm:flex-row");
    expect(screen.getByTestId("page-header-action")).toHaveClass(
      "w-full",
      "sm:w-auto",
    );
  });
});
