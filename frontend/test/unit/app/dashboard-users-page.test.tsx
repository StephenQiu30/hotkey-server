import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

const mocks = vi.hoisted(() => ({
  getUsers: vi.fn(),
  patchUsersId: vi.fn(),
  deleteUsersId: vi.fn(),
  postUsersIdRestore: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/identity", () => mocks);

import UsersPage from "@/app/dashboard/users/page";

const users = [
  { id: 1, email: "admin@example.test", display_name: "Admin", role: "admin", status: "active" },
  { id: 2, email: "editor@example.test", display_name: "Editor", role: "editor", status: "active" },
  { id: 3, email: "deleted@example.test", display_name: "Deleted", role: "viewer", status: "disabled", deleted_at: "2026-08-01T00:00:00Z" },
] as HotKeyAPI.UserResponse[];

function setRole(role: UserRole) {
  useAuthStore.setState({
    status: AuthStatus.Authenticated,
    user: { id: 1, email: "actor@example.test", role },
    error: null,
  });
}

describe("UsersPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getUsers.mockResolvedValue({ data: users });
    mocks.patchUsersId.mockResolvedValue({ data: { ...users[1], role: "viewer" } });
    mocks.deleteUsersId.mockResolvedValue({ data: { ...users[1], deleted_at: "2026-08-07T00:00:00Z" } });
    mocks.postUsersIdRestore.mockResolvedValue({ data: { ...users[2], deleted_at: undefined } });
    setRole(UserRole.Admin);
  });

  it("does not render or load user management for non-admins", () => {
    setRole(UserRole.Editor);
    const { container } = render(<UsersPage />);

    expect(container).toBeEmptyDOMElement();
    expect(mocks.getUsers).not.toHaveBeenCalled();
  });

  it("filters users and exposes audited lifecycle actions", async () => {
    render(<UsersPage />);
    const user = userEvent.setup();

    expect(await screen.findByText("editor@example.test")).toBeInTheDocument();
    await user.type(screen.getByRole("searchbox", { name: "搜索用户" }), "deleted@");
    expect(screen.getByText("deleted@example.test")).toBeInTheDocument();
    expect(screen.queryByText("editor@example.test")).not.toBeInTheDocument();

    await user.clear(screen.getByRole("searchbox", { name: "搜索用户" }));
    await user.click(screen.getByRole("button", { name: "恢复 deleted@example.test" }));
    await waitFor(() => expect(mocks.postUsersIdRestore).toHaveBeenCalledWith({ id: 3 }));

    await user.click(screen.getByRole("button", { name: "删除 editor@example.test" }));
    expect(screen.getByRole("alertdialog", { name: "删除用户？" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "确认删除" }));
    await waitFor(() => expect(mocks.deleteUsersId).toHaveBeenCalledWith({ id: 2 }));
  });

  it("changes a user status through the generated admin API", async () => {
    render(<UsersPage />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: "禁用 editor@example.test" }));
    await waitFor(() =>
      expect(mocks.patchUsersId).toHaveBeenCalledWith({ id: 2 }, { status: "disabled" }),
    );
  });

  it("changes a user role through the generated admin API", async () => {
    render(<UsersPage />);
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("combobox", { name: "设置 editor@example.test 的角色" }),
    );
    await user.click(screen.getByRole("option", { name: "查看者" }));

    await waitFor(() =>
      expect(mocks.patchUsersId).toHaveBeenCalledWith({ id: 2 }, { role: "viewer" }),
    );
  });

  it("offers a retry after the user list fails to load", async () => {
    mocks.getUsers
      .mockRejectedValueOnce(new Error("network unavailable"))
      .mockResolvedValueOnce({ data: users });
    render(<UsersPage />);
    const user = userEvent.setup();

    expect(await screen.findByText("network unavailable")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "重试" }));

    expect(await screen.findByText("editor@example.test")).toBeInTheDocument();
    expect(mocks.getUsers).toHaveBeenCalledTimes(2);
  });
});
