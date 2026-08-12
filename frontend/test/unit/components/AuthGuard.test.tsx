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
    "/dashboard/sources",
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
    await waitFor(() => {
      expect(navigation.replace).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("renders an administrator-only route for an administrator", () => {
    navigation.pathname = "/dashboard/sources";
    navigation.query = "";
    navigation.auth = {
      status: AuthStatus.Authenticated,
      user: { role: UserRole.Admin },
    };
    window.history.replaceState({}, "", navigation.pathname);

    render(
      <AuthGuard>
        <main>administrator page</main>
      </AuthGuard>,
    );

    expect(screen.getByText("administrator page")).toBeInTheDocument();
    expect(navigation.replace).not.toHaveBeenCalled();
  });
});
