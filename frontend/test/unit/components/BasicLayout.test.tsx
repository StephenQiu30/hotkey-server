import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import BasicLayout from "@/layouts/BasicLayout";

vi.mock("@/components/dashboard/TopNav", () => ({
  default: () => (
    <header data-layout-header>
      <nav aria-label="主导航" />
    </header>
  ),
}));

vi.mock("@/components/notifications/RealtimeNotifications", () => ({
  RealtimeNotifications: () => null,
}));

describe("BasicLayout", () => {
  it("provides a stable shell with one scrollable main landmark", () => {
    render(
      <BasicLayout menuItems={[]}>
        <section>工作台内容</section>
      </BasicLayout>
    );

    expect(screen.getByTestId("basic-layout")).toHaveClass(
      "flex",
      "h-dvh",
      "flex-col",
      "overflow-hidden",
    );
    expect(screen.getByRole("link", { name: "跳到主要内容" })).toHaveAttribute(
      "href",
      "#main-content"
    );
    expect(screen.getAllByRole("main")).toHaveLength(1);
    expect(screen.getByRole("main")).toHaveAttribute("data-layout-scroll-region");
    expect(screen.getByRole("main")).toHaveClass(
      "min-h-0",
      "flex-1",
      "overflow-y-auto",
    );
    expect(screen.getByRole("main")).toHaveAttribute("id", "main-content");
    expect(screen.getByRole("main")).toHaveAttribute("tabindex", "-1");
    expect(screen.getByRole("banner")).toHaveAttribute("data-layout-header");
    expect(screen.getByRole("contentinfo")).toHaveAttribute(
      "data-layout-footer",
    );
  });
});
