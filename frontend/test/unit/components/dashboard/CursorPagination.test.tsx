import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  CursorPagination,
  hasNextCursor,
} from "@/components/dashboard/CursorPagination";

describe("CursorPagination", () => {
  afterEach(() => {
    cleanup();
  });

  it("shows next navigation when the server returns a cursor", async () => {
    const user = userEvent.setup();
    const onNext = vi.fn();

    render(
      <CursorPagination
        hasNext
        onNext={onNext}
        onPrevious={vi.fn()}
        page={1}
      />,
    );

    expect(screen.getByText("第 1 页")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "上一页" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "下一页" }));
    expect(onNext).toHaveBeenCalledOnce();
  });

  it("treats a missing or blank server cursor as the end of the list", () => {
    expect(hasNextCursor(undefined)).toBe(false);
    expect(hasNextCursor(null)).toBe(false);
    expect(hasNextCursor("")).toBe(false);
    expect(hasNextCursor("   ")).toBe(false);
    expect(hasNextCursor("cursor-2")).toBe(true);
    expect(hasNextCursor(42)).toBe(true);
  });

  it("lets the user change the page size", async () => {
    const user = userEvent.setup();
    const onPageSizeChange = vi.fn();

    render(
      <CursorPagination
        hasNext={false}
        onNext={vi.fn()}
        onPageSizeChange={onPageSizeChange}
        onPrevious={vi.fn()}
        page={1}
        pageSize={20}
      />,
    );

    const pageSizeSelect = screen.getByRole("combobox", { name: "每页条数" });
    expect(pageSizeSelect).toHaveTextContent("20 条");
    await user.click(pageSizeSelect);
    await user.click(screen.getByRole("option", { name: "50 条" }));
    expect(onPageSizeChange).toHaveBeenCalledWith(50);
  });

  it("keeps previous navigation available after moving past the first page", () => {
    const onPrevious = vi.fn();

    render(
      <CursorPagination
        hasNext={false}
        onNext={vi.fn()}
        onPrevious={onPrevious}
        page={2}
      />,
    );

    expect(screen.getByRole("button", { name: "下一页" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "上一页" })).toBeEnabled();
  });

  it("does not render for a single page", () => {
    render(
      <CursorPagination
        hasNext={false}
        onNext={vi.fn()}
        onPrevious={vi.fn()}
        page={1}
      />,
    );

    expect(screen.queryByText("第 1 页")).not.toBeInTheDocument();
  });
});
