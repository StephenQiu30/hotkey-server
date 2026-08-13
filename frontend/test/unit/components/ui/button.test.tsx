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

  it("gives secondary outline actions a visible one-pixel boundary", () => {
    render(<Button variant="outline">刷新热点</Button>);

    expect(screen.getByRole("button", { name: "刷新热点" })).toHaveClass(
      "border",
      "border-border",
      "bg-background",
    );
  });
});
