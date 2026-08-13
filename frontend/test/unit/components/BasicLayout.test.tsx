import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import BasicLayout from "@/layouts/BasicLayout";

vi.mock("@/components/dashboard/TopNav", () => ({
  default: () => <nav aria-label="主导航" />,
}));

vi.mock("@/components/notifications/RealtimeNotifications", () => ({
  RealtimeNotifications: () => null,
}));

describe("BasicLayout", () => {
  it("provides one named main landmark and a keyboard skip link", () => {
    render(
      <BasicLayout menuItems={[]}>
        <section>工作台内容</section>
      </BasicLayout>
    );

    expect(screen.getByRole("link", { name: "跳到主要内容" })).toHaveAttribute(
      "href",
      "#main-content"
    );
    expect(screen.getAllByRole("main")).toHaveLength(1);
    expect(screen.getByRole("main")).toHaveAttribute("id", "main-content");
    expect(screen.getByRole("main")).toHaveAttribute("tabindex", "-1");
  });
});
