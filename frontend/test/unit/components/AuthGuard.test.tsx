import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthStatus, UserRole } from "@/lib/domainEnums";

const navigation = vi.hoisted(() => ({
  pathname: "/dashboard/events",
  query: "status=active",
  replace: vi.fn(),
  auth: {
    status: "unauthenticated",
    user: null as { role: string } | null,
  },
}));

vi.mock("next/navigation", () => ({
  usePathname: () => navigation.pathname,
  useRouter: () => ({ replace: navigation.replace }),
}));

vi.mock("@/stores/authStore", () => ({
  useAuthStore: (
    selector: (state: {
      status: string;
      user: { role: string } | null;
    }) => unknown,
  ) => selector(navigation.auth),
}));

import AuthGuard from "@/components/AuthGuard";

describe("AuthGuard", () => {
  beforeEach(() => {
    cleanup();
    navigation.pathname = "/dashboard/events";
    navigation.query = "status=active";
    navigation.auth = {
      status: AuthStatus.Unauthenticated,
      user: null,
    };
    navigation.replace.mockReset();
    window.history.replaceState({}, "", `/dashboard/events?${navigation.query}`);
  });

  it("announces the initializing state instead of rendering a blank screen", () => {
    navigation.auth = {
      status: AuthStatus.Initializing,
      user: null,
    };

    render(
      <AuthGuard>
        <main>private</main>
      </AuthGuard>,
    );

    expect(
      screen.getByRole("status", { name: "正在验证访问权限" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("private")).not.toBeInTheDocument();
  });

  it("redirects an unauthenticated visitor while preserving the safe return target", async () => {
    render(
      <AuthGuard>
        <main>private</main>
      </AuthGuard>,
    );

    await waitFor(() => {
      expect(navigation.replace).toHaveBeenCalledWith(
        "/login?redirect=%2Fdashboard%2Fevents%3Fstatus%3Dactive",
      );
    });
  });

  it.each([
    "/dashboard/users",
    "/dashboard/governance",
  ])("redirects a non-admin away from %s without rendering the page", async (pathname) => {
    navigation.pathname = pathname;
    navigation.query = "";
    navigation.auth = {
      status: AuthStatus.Authenticated,
      user: { role: UserRole.Editor },
    };
    window.history.replaceState({}, "", pathname);

    render(
      <AuthGuard>
        <main>administrator page</main>
      </AuthGuard>,
    );

    expect(screen.queryByText("administrator page")).not.toBeInTheDocument();
    expect(
      screen.getByRole("alert", { name: "权限不足" }),
    ).toHaveTextContent("当前账号没有访问此页面的权限");
    await waitFor(() => {
      expect(navigation.replace).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("keeps a recognized analyst out of administrator-only routes", async () => {
    navigation.pathname = "/dashboard/users";
    navigation.query = "";
    navigation.auth = {
      status: AuthStatus.Authenticated,
      user: { role: "analyst" },
    };
    window.history.replaceState({}, "", navigation.pathname);

    render(
      <AuthGuard>
        <main>administrator page</main>
      </AuthGuard>,
    );

    expect(screen.queryByText("administrator page")).not.toBeInTheDocument();
    expect(
      screen.getByRole("alert", { name: "权限不足" }),
    ).toHaveTextContent("当前账号没有访问此页面的权限");
    await waitFor(() => {
      expect(navigation.replace).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("allows an analyst to open the safe source directory", () => {
    navigation.pathname = "/dashboard/sources";
    navigation.query = "";
    navigation.auth = {
      status: AuthStatus.Authenticated,
      user: { role: UserRole.Analyst },
    };
    window.history.replaceState({}, "", navigation.pathname);

    render(
      <AuthGuard>
        <main>safe source directory</main>
      </AuthGuard>,
    );

    expect(screen.getByText("safe source directory")).toBeInTheDocument();
    expect(navigation.replace).not.toHaveBeenCalled();
  });
});
