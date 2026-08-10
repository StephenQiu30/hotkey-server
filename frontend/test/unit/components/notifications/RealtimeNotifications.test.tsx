import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RealtimeNotifications } from "@/components/notifications/RealtimeNotifications";
import { setAccessToken } from "@/lib/authSession";
import { AuthStatus, UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";
import { useNotificationStore } from "@/stores/notificationStore";

const mocks = vi.hoisted(() => ({
  getNotifications: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/services/hotkey/hotkey-server/notifications", () => ({
  getNotifications: mocks.getNotifications,
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
    setAccessToken("test-access-token", 900);
  });

  it("opens a native browser stream, ingests one frame and shows a toast", async () => {
    const body = [
      "id: 4",
      "event: micro_event.updated",
      'data: {"id":4,"version":1,"monitor_id":2,"event_type":"micro_event.updated","resource_type":"micro_event","resource_id":9,"resource_version":1,"occurred_at":"2026-08-08T00:00:00Z","created_at":"2026-08-08T00:00:00Z","title":"事件更新","summary":"新增独立正文谱系","resource_status":"active","deep_link":"/dashboard/events?event=9"}',
      "",
      "",
    ].join("\n");
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      new Response(new ReadableStream<Uint8Array>({
        start(controller) {
          controller.enqueue(new TextEncoder().encode(body));
          controller.close();
        },
      }), { status: 200, headers: { "Content-Type": "text/event-stream" } }),
    );

    render(<RealtimeNotifications />);

    await waitFor(() => expect(useNotificationStore.getState().items).toHaveLength(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/notifications/stream?after_id=0",
      expect.objectContaining({ method: "GET", cache: "no-store", credentials: "include" }),
    );
    expect(mocks.toast).toHaveBeenCalledWith("事件更新", { description: "新增独立正文谱系" });
  });
});
