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
import { useNotificationStore } from "@/stores/notificationStore";
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
    window.history.replaceState({}, "", "/dashboard");
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
    useNotificationStore.setState({ unreadCount: 3 });
  });

  it("keeps the primary product areas in a restrained top navigation", () => {
    render(
      <TopNav
        menuItems={[{ path: "/dashboard", name: "概览", icon: <Activity /> }]}
        adminMenuItems={[
          { path: "/dashboard/sources", name: "来源管理", icon: <Database /> },
        ]}
      />
    );

    expect(screen.getByRole("banner")).toHaveAttribute("data-top-nav");
    expect(screen.getByRole("navigation", { name: "主导航" })).toHaveClass(
      "xl:flex"
    );
    expect(screen.getByRole("link", { name: /概览/ })).toHaveAttribute(
      "aria-current",
      "page"
    );
    expect(
      screen.getByRole("link", { name: "通知，3 条未读" })
    ).toHaveAttribute("href", "/dashboard/notifications");
    expect(
      screen.getByRole("button", { name: "账户菜单" })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "切换到暗色模式" })
    ).toBeInTheDocument();
  });

  it("submits global search to the event workspace", () => {
    render(<TopNav menuItems={[]} />);

    fireEvent.change(screen.getByRole("searchbox", { name: "搜索信号" }), {
      target: { value: "化工安全" },
    });
    fireEvent.submit(screen.getByRole("search"));

    expect(navigationMocks.push).toHaveBeenCalledWith(
      "/dashboard/contents?q=%E5%8C%96%E5%B7%A5%E5%AE%89%E5%85%A8"
    );
  });

  it("resynchronizes search after same-path browser history navigation", async () => {
    window.history.replaceState({}, "", "/dashboard/contents?q=旧查询");
    render(<TopNav menuItems={[]} />);

    expect(await screen.findByDisplayValue("旧查询")).toBeInTheDocument();
    window.history.pushState({}, "", "/dashboard/contents?q=新查询");
    fireEvent.popState(window);

    expect(await screen.findByDisplayValue("新查询")).toBeInTheDocument();
  });

  it("keeps operational pages in the account menu for administrators", async () => {
    const user = userEvent.setup();
    render(
      <TopNav
        menuItems={[]}
        adminMenuItems={[
          { path: "/dashboard/sources", name: "来源管理", icon: <Database /> },
        ]}
      />
    );

    await user.click(screen.getByRole("button", { name: "账户菜单" }));
    expect(
      await screen.findByRole("menuitem", { name: /来源管理/ })
    ).toBeInTheDocument();
  }, 15_000);

  it("hides administrator-only menu items from editors", async () => {
    useAuthStore.setState((state) => ({
      ...state,
      user: { ...state.user!, role: UserRole.Editor },
    }));
    const user = userEvent.setup();
    render(
      <TopNav
        menuItems={[]}
        adminMenuItems={[
          {
            path: "/dashboard/users",
            name: "用户与权限",
            icon: <Activity />,
            roles: [UserRole.Admin],
          },
          { path: "/dashboard/contents", name: "采集内容", icon: <Database /> },
        ]}
      />
    );

    await user.click(screen.getByRole("button", { name: "账户菜单" }));
    expect(
      screen.queryByRole("menuitem", { name: /用户与权限/ })
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("menuitem", { name: /采集内容/ })
    ).toBeInTheDocument();
  }, 15_000);

  it("hides administrator-only primary routes from desktop and mobile navigation", async () => {
    useAuthStore.setState((state) => ({
      ...state,
      user: { ...state.user!, role: UserRole.Viewer },
    }));
    const user = userEvent.setup();
    render(
      <TopNav
        menuItems={[
          { path: "/dashboard", name: "概览", icon: <Activity /> },
          {
            path: "/dashboard/sources",
            name: "来源",
            icon: <Database />,
            roles: [UserRole.Admin],
          },
        ]}
      />
    );

    expect(
      screen
        .queryByRole("navigation", { name: "主导航" })
        ?.querySelector('a[href="/dashboard/sources"]')
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "打开导航" }));
    expect(
      screen.queryByRole("link", { name: /来源/ })
    ).not.toBeInTheDocument();
  });

  it("opens mobile navigation as an accessible sheet", async () => {
    const user = userEvent.setup();
    render(
      <TopNav
        menuItems={[{ path: "/dashboard", name: "概览", icon: <Activity /> }]}
      />
    );

    await user.click(screen.getByRole("button", { name: "打开导航" }));

    expect(
      await screen.findByRole("dialog", { name: "工作区导航" })
    ).toBeInTheDocument();
    expect(
      screen.getByRole("navigation", { name: "移动导航" })
    ).toBeInTheDocument();

    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "工作区导航" })
      ).not.toBeInTheDocument();
    });
  });

  it("shows administrator destinations inside the mobile sheet", async () => {
    const user = userEvent.setup();
    render(
      <TopNav
        menuItems={[{ path: "/dashboard", name: "概览", icon: <Activity /> }]}
        adminMenuItems={[
          {
            path: "/dashboard/users",
            name: "用户与权限",
            icon: <Database />,
            roles: [UserRole.Admin],
          },
        ]}
      />
    );

    await user.click(screen.getByRole("button", { name: "打开导航" }));
    expect(screen.getByRole("link", { name: /用户与权限/ })).toHaveAttribute(
      "href",
      "/dashboard/users"
    );
  });
});
