import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

const params = vi.hoisted(() => ({ id: "7" }));

vi.mock("next/navigation", () => ({
  useParams: () => params,
}));

vi.mock("@/components/dashboard/MonitorIntentWorkspace", () => ({
  MonitorIntentWorkspace: ({ canAdmin, monitorID }: { canAdmin: boolean; monitorID: number }) => (
    <div data-testid="intent-workspace">{`${monitorID}:${canAdmin ? "admin" : "editor"}`}</div>
  ),
}));

import MonitorIntentPage from "@/app/dashboard/settings/monitors/[id]/intent/page";

function setRole(role: UserRole) {
  useAuthStore.setState({
    status: AuthStatus.Authenticated,
    user: { id: 1, email: `${role}@example.test`, role },
    error: null,
  });
}

describe("MonitorIntentPage", () => {
  afterEach(() => {
    cleanup();
    params.id = "7";
  });

  it("opens the exact monitor workspace for an administrator", () => {
    setRole(UserRole.Admin);
    render(<MonitorIntentPage />);

    expect(screen.getByTestId("intent-workspace")).toHaveTextContent("7:admin");
    expect(screen.getByRole("link", { name: "返回热点监控" })).toHaveAttribute(
      "href",
      "/dashboard/settings",
    );
    expect(screen.getByRole("link", { name: "查看匹配判定" })).toHaveAttribute(
      "href",
      "/dashboard/settings/monitors/7/matches",
    );
  });

  it("fails closed for viewers and malformed monitor identities", () => {
    setRole(UserRole.Viewer);
    render(<MonitorIntentPage />);
    expect(screen.getByText("当前角色不能编辑语义意图")).toBeInTheDocument();
    expect(screen.queryByTestId("intent-workspace")).not.toBeInTheDocument();

    cleanup();
    setRole(UserRole.Admin);
    params.id = "not-an-id";
    render(<MonitorIntentPage />);
    expect(screen.getByText("监控编号无效")).toBeInTheDocument();
  });
});
