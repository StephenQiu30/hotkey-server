import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PageShell } from "@/layouts/PageShell";

describe("PageShell", () => {
  it("owns the shared dashboard page frame", () => {
    render(<PageShell>页面内容</PageShell>);

    expect(screen.getByText("页面内容")).toHaveClass("app-page");
    expect(screen.getByText("页面内容")).toHaveAttribute(
      "data-slot",
      "page-shell",
    );
  });

  it("can center loading and error states without duplicating layout classes", () => {
    render(<PageShell align="center">正在加载</PageShell>);

    expect(screen.getByText("正在加载")).toHaveClass(
      "flex",
      "min-h-full",
      "items-center",
      "justify-center",
    );
  });

  it("can style an existing semantic landmark through Radix Slot", () => {
    render(
      <PageShell asChild>
        <main>权限检查</main>
      </PageShell>,
    );

    expect(screen.getByRole("main")).toHaveClass("app-page");
    expect(screen.getByRole("main")).toHaveAttribute(
      "data-slot",
      "page-shell",
    );
  });
});
