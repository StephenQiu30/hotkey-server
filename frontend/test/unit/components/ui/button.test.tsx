import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Button } from "@/components/ui/button";

describe("Button", () => {
  it("uses the official destructive theme tokens", () => {
    render(<Button variant="destructive">确认删除</Button>);

    expect(screen.getByRole("button", { name: "确认删除" })).toHaveClass(
      "bg-destructive",
      "text-destructive-foreground",
    );
  });

  it("uses a low-contrast color surface for secondary outline actions", () => {
    render(<Button variant="outline">刷新热点</Button>);

    expect(screen.getByRole("button", { name: "刷新热点" })).toHaveClass(
      "bg-secondary/70",
      "text-secondary-foreground",
    );
    expect(screen.getByRole("button", { name: "刷新热点" })).not.toHaveClass(
      "border",
      "border-border",
      "[box-shadow:var(--shadow-border)]",
    );
  });

  it("keeps disabled actions legible without opacity blending", () => {
    render(<Button variant="outline" disabled>正在同步</Button>);

    expect(screen.getByRole("button", { name: "正在同步" })).toHaveClass(
      "disabled:bg-muted",
      "disabled:text-foreground",
      "disabled:opacity-100",
    );
  });

  it("does not animate between accessible foreground and background color pairs", () => {
    render(<Button disabled>搜索</Button>);

    const button = screen.getByRole("button", { name: "搜索" });
    expect(button).toHaveClass("transition-[box-shadow,opacity,transform]");
    expect(button).not.toHaveClass("transition-[background-color,color,box-shadow,opacity,transform]");
  });
});
