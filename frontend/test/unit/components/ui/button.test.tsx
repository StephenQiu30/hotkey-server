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
});
