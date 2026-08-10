import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

const params = vi.hoisted(() => ({ id: "7" }));

vi.mock("next/navigation", () => ({ useParams: () => params }));
vi.mock("@/components/dashboard/DocumentMatchWorkspace", () => ({
  DocumentMatchWorkspace: ({ canReview, monitorID }: { canReview: boolean; monitorID: number }) => (
    <div data-testid="document-match-workspace">{`${monitorID}:${canReview ? "review" : "read"}`}</div>
  ),
}));

import DocumentMatchesPage from "@/app/dashboard/settings/monitors/[id]/matches/page";

function setRole(role: UserRole) {
  useAuthStore.setState({
    status: AuthStatus.Authenticated,
    user: { id: 1, email: `${role}@example.test`, role },
    error: null,
  });
}

describe("DocumentMatchesPage", () => {
  afterEach(() => {
    cleanup();
    params.id = "7";
  });

  it("allows viewers to inspect exact matches and editors to review them", () => {
    setRole(UserRole.Viewer);
    render(<DocumentMatchesPage />);
    expect(screen.getByTestId("document-match-workspace")).toHaveTextContent("7:read");

    cleanup();
    setRole(UserRole.Editor);
    render(<DocumentMatchesPage />);
    expect(screen.getByTestId("document-match-workspace")).toHaveTextContent("7:review");
    expect(screen.getByRole("link", { name: "编辑语义意图" })).toHaveAttribute(
      "href",
      "/dashboard/settings/monitors/7/intent",
    );
  });

  it("fails closed for malformed monitor identities", () => {
    setRole(UserRole.Admin);
    params.id = "bad";
    render(<DocumentMatchesPage />);
    expect(screen.getByText("监控编号无效")).toBeInTheDocument();
    expect(screen.queryByTestId("document-match-workspace")).not.toBeInTheDocument();
  });
});
