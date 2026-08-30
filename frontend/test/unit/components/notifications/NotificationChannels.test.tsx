import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { NotificationChannels } from "@/components/notifications/NotificationChannels";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";
import { useNotificationStore } from "@/stores/notificationStore";

describe("NotificationChannels", () => {
  beforeEach(() => {
    useAuthStore.setState({
      status: AuthStatus.Authenticated,
      user: {
        id: 7,
        email: "viewer@example.test",
        display_name: "Viewer",
        role: UserRole.Viewer,
        status: "active",
      },
      error: null,
    });
    useNotificationStore.getState().reset();
  });

  it("uses the borderless secondary badge treatment for a live WebSocket connection", () => {
    useNotificationStore.setState({ transport: "live" });

    render(<NotificationChannels />);

    const status = screen.getByText("WebSocket 已连接");
    expect(status).toHaveClass("bg-secondary/70", "text-secondary-foreground");
    expect(status).not.toHaveClass("bg-primary", "text-primary-foreground");
  });
});
