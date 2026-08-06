import { render, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({
  pathname: "/dashboard/events",
  query: "status=active",
  replace: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  usePathname: () => navigation.pathname,
  useRouter: () => ({ replace: navigation.replace }),
}));

vi.mock("@/stores/authStore", () => ({
  useAuthStore: (selector: (state: { status: string }) => unknown) =>
    selector({ status: "unauthenticated" }),
}));

import AuthGuard from "@/components/AuthGuard";

describe("AuthGuard", () => {
  beforeEach(() => {
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
});
