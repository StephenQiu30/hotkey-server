import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Activity, Database } from "lucide-react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import TopNav from "@/components/dashboard/TopNav";
import { useAuthStore } from "@/stores/authStore";
import { AuthStatus, UserRole } from "@/lib/domainEnums";

const navigationMocks = vi.hoisted(() => ({
  pathname: "/dashboard",
  push: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  usePathname: () => navigationMocks.pathname,
  useRouter: () => ({ push: navigationMocks.push }),
}));

describe("TopNav", () => {
  afterEach(cleanup);

  beforeEach(() => {
    navigationMocks.pathname = "/dashboard";
    navigationMocks.push.mockReset();
    useAuthStore.setState({
      status: AuthStatus.Authenticated,
      user: {
        id: 2,
        email: "qa@example.test",
        display_name: "QA",
        role: UserRole.Admin,
        status: "active",
      },
      error: null,
    });
  });

  it("keeps the primary product areas in a restrained top navigation", () => {
    render(
      <TopNav
        alertCount={3}
        menuItems={[{ path: "/dashboard", name: "概览", icon: <Activity /> }]}
        adminMenuItems={[
          { path: "/dashboard/sources", name: "来源管理", icon: <Database /> },
        ]}
      />,
    );

    expect(screen.getByRole("banner")).toHaveAttribute("data-top-nav");
    expect(screen.getByRole("navigation", { name: "主导航" })).toHaveClass(
      "lg:flex",
    );
    expect(screen.getByRole("link", { name: /概览/ })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "告警，3 条待处理" })).toHaveAttribute(
      "href",
      "/dashboard/alerts",
    );
    expect(screen.getByRole("button", { name: "账户菜单" })).toBeInTheDocument();
  });

  it("submits global search to the event workspace", () => {
    render(<TopNav menuItems={[]} />);

    fireEvent.change(screen.getByRole("searchbox", { name: "搜索事件或监控" }), {
      target: { value: "化工安全" },
    });
    fireEvent.submit(screen.getByRole("search"));

    expect(navigationMocks.push).toHaveBeenCalledWith(
      "/dashboard/events?q=%E5%8C%96%E5%B7%A5%E5%AE%89%E5%85%A8",
    );
  });

  it("keeps operational pages in the account menu for administrators", async () => {
    const user = userEvent.setup();
    render(
      <TopNav
        menuItems={[]}
        adminMenuItems={[
          { path: "/dashboard/sources", name: "来源管理", icon: <Database /> },
        ]}
      />,
    );

    await user.click(screen.getByRole("button", { name: "账户菜单" }));
    expect(await screen.findByRole("menuitem", { name: /来源管理/ })).toBeInTheDocument();
  });

  it("opens mobile navigation as an accessible sheet", async () => {
    const user = userEvent.setup();
    render(
      <TopNav
        menuItems={[{ path: "/dashboard", name: "概览", icon: <Activity /> }]}
      />,
    );

    await user.click(screen.getByRole("button", { name: "打开导航" }));

    expect(await screen.findByRole("dialog", { name: "工作区导航" })).toBeInTheDocument();
    expect(
      screen.getByRole("navigation", { name: "移动导航" }),
    ).toBeInTheDocument();

    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "工作区导航" })).not.toBeInTheDocument();
    });
  });
});
