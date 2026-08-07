import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RealtimeNotifications } from "@/components/notifications/RealtimeNotifications";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";
import { useNotificationStore } from "@/stores/notificationStore";

const mocks = vi.hoisted(() => ({
  getNotifications: vi.fn(),
  getNotificationsStream: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/notifications", () => ({
  getNotifications: mocks.getNotifications,
  getNotificationsStream: mocks.getNotificationsStream,
}));

vi.mock("sonner", () => ({ toast: mocks.toast }));

describe("RealtimeNotifications", () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useNotificationStore.getState().reset();
    useAuthStore.setState({
      status: AuthStatus.Authenticated,
      user: { id: 7, email: "viewer@example.test", display_name: "Viewer", role: UserRole.Viewer, status: "active" },
      error: null,
    });
    mocks.getNotifications.mockResolvedValue({ data: { items: [], next_after_id: 0 } });
  });

  it("uses the generated stream client, ingests one frame and shows a toast", async () => {
    const body = [
      "id: 4",
      "event: event.updated",
      'data: {"id":4,"event_type":"event.updated","resource_type":"event","resource_id":9,"audience":"viewer","occurred_at":"2026-08-08T00:00:00Z","payload":{"title":"事件更新","summary":"热度上升"}}',
      "",
      "",
    ].join("\n");
    mocks.getNotificationsStream.mockResolvedValueOnce(
      new ReadableStream<Uint8Array>({
        start(controller) {
          controller.enqueue(new TextEncoder().encode(body));
          controller.close();
        },
      }),
    );

    render(<RealtimeNotifications />);

    await waitFor(() => expect(useNotificationStore.getState().items).toHaveLength(1));
    expect(mocks.getNotificationsStream).toHaveBeenCalledWith(
      { after_id: 0 },
      expect.objectContaining({ adapter: "fetch", responseType: "stream", timeout: 0 }),
    );
    expect(mocks.toast).toHaveBeenCalledWith("事件更新", { description: "热度上升" });
  });
});
