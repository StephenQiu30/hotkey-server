import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";

describe("ConfirmDeleteDialog", () => {
  it("explains soft deletion and only confirms after an explicit click", () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmDeleteDialog
        open
        onOpenChange={vi.fn()}
        title="删除监控"
        description="监控将从工作区隐藏，历史事件与报告仍会保留。"
        resourceName="AI Agent 创建工具"
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByText("AI Agent 创建工具")).toBeInTheDocument();
    expect(screen.getByText(/历史事件与报告仍会保留/)).toBeInTheDocument();
    expect(onConfirm).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "确认删除" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});
