import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { vi } from "vitest";
import { CollectionWorkspace } from "@/components/dashboard/CollectionWorkspace";

describe("CollectionWorkspace", () => {
  afterEach(() => {
    cleanup();
  });

  it("shows recent runs and normalized content as separate pipeline stages", () => {
    render(
      <CollectionWorkspace
        runs={[
          {
            id: 21,
            status: "failed",
            candidate_count: 3,
            accepted_count: 0,
            rejected_count: 3,
            error_code: "destination_not_permitted",
          },
          {
            id: 20,
            status: "succeeded",
            candidate_count: 5,
            accepted_count: 4,
            rejected_count: 1,
          },
        ]}
        contents={[
          {
            id: 4,
            title: "Agent systems are changing software development",
            source_name: "arXiv · Artificial Intelligence",
            canonical_url: "https://example.test/paper",
            fetched_at: "2026-07-18T04:00:00Z",
          },
        ]}
      />
    );

    expect(
      screen.getByRole("heading", { name: "采集批次（当前页）" })
    ).toBeInTheDocument();
    expect(screen.getByText("destination_not_permitted")).toBeInTheDocument();
    expect(screen.getByTestId("collection-pipeline")).toHaveClass(
      "lg:grid-cols-2",
    );
    expect(screen.getByTestId("collection-pipeline")).toHaveClass(
      "items-stretch",
    );
    expect(
      screen.getByRole("link", {
        name: "阅读归档：Agent systems are changing software development",
      }),
    ).toHaveAttribute("href", "/dashboard/contents/4");
    expect(screen.getByRole("link", { name: "访问原站" })).toHaveAttribute(
      "href",
      "https://example.test/paper"
    );
  });

  it("explains when the scheduler has not created any run", () => {
    render(<CollectionWorkspace runs={[]} contents={[]} />);

    expect(screen.getByText("尚未产生采集批次")).toBeInTheDocument();
    expect(
      screen.getByText("发布监控后仍长期停留在这里，通常表示后台调度器未创建任务。")
    ).toBeInTheDocument();
  });

  it("only exposes entry points supported by each content record", () => {
    render(
      <CollectionWorkspace
        runs={[]}
        contents={[
          { title: "Missing internal id", canonical_url: "https://example.test/external" },
          { id: 9, title: "Missing canonical URL" },
        ]}
      />,
    );

    expect(
      screen.queryByRole("link", { name: "阅读归档：Missing internal id" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "访问原站" })).toHaveAttribute(
      "href",
      "https://example.test/external",
    );
    expect(
      screen.getByRole("link", { name: "阅读归档：Missing canonical URL" }),
    ).toHaveAttribute("href", "/dashboard/contents/9");
  });

  it("exposes content deletion only to callers with management permission", async () => {
    const onDelete = vi.fn();
    const content = { id: 9, title: "Content to remove" };
    const { rerender } = render(
      <CollectionWorkspace
        canManage
        contents={[content]}
        onDelete={onDelete}
        runs={[]}
      />,
    );

    await userEvent.setup().click(
      screen.getByRole("button", { name: "删除内容：Content to remove" }),
    );
    expect(onDelete).toHaveBeenCalledWith(content);

    rerender(<CollectionWorkspace contents={[content]} runs={[]} />);
    expect(
      screen.queryByRole("button", { name: "删除内容：Content to remove" }),
    ).not.toBeInTheDocument();
  });
});
