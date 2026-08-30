import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Card } from "@/components/ui/card";
import { Surface } from "@/components/ui/surface";

describe("Surface", () => {
  it("keeps the legacy ring variant borderless without simulating an outline", () => {
    render(<Surface variant="ring">证据面板</Surface>);

    expect(screen.getByText("证据面板")).toHaveClass(
      "rounded-lg",
      "[box-shadow:var(--shadow-card)]",
    );
    expect(screen.getByText("证据面板")).not.toHaveClass(
      "border",
      "[box-shadow:var(--shadow-border)]",
    );
  });

  it("shares the same surface variants with cards", () => {
    render(<Card variant="subtle">监控状态</Card>);

    expect(screen.getByText("监控状态")).toHaveClass(
      "bg-muted/40",
      "shadow-none",
    );
  });
});
