import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import DashboardError from "@/app/dashboard/error";
import DashboardLoading from "@/app/dashboard/loading";

describe("dashboard route states", () => {
  it("announces route loading without a blank screen", () => {
    render(<DashboardLoading />);
    expect(screen.getByRole("status", { name: "正在加载工作台" })).toBeInTheDocument();
  });

  it("hides internal errors and delegates recovery to the route boundary", () => {
    const reset = vi.fn();
    render(<DashboardError error={new Error("private stack detail")} reset={reset} />);
    expect(screen.getByText("工作台暂时不可用")).toBeInTheDocument();
    expect(screen.queryByText("private stack detail")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "重试当前页面" }));
    expect(reset).toHaveBeenCalledTimes(1);
  });
});
