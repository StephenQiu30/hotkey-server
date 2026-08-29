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

  it("gives secondary actions a borderless shadow ring", () => {
    render(<Button variant="outline">刷新热点</Button>);

    expect(screen.getByRole("button", { name: "刷新热点" })).toHaveClass(
      "bg-background",
      "[box-shadow:var(--shadow-border)]",
    );
    expect(screen.getByRole("button", { name: "刷新热点" })).not.toHaveClass(
      "border",
      "border-border",
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
});
