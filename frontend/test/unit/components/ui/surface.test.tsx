import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Card } from "@/components/ui/card";
import { Surface } from "@/components/ui/surface";

describe("Surface", () => {
  it("uses the canonical Vercel ring without a CSS border", () => {
    render(<Surface variant="ring">证据面板</Surface>);

    expect(screen.getByText("证据面板")).toHaveClass(
      "rounded-lg",
      "[box-shadow:var(--shadow-border)]",
    );
    expect(screen.getByText("证据面板")).not.toHaveClass("border");
  });

  it("shares the same surface variants with cards", () => {
    render(<Card variant="subtle">监控状态</Card>);

    expect(screen.getByText("监控状态")).toHaveClass(
      "bg-muted/30",
      "[box-shadow:var(--shadow-border)]",
    );
  });
});
