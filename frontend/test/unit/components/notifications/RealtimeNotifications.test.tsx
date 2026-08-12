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
  const originalWebSocket = globalThis.WebSocket;

  afterEach(() => {
    cleanup();
    globalThis.WebSocket = originalWebSocket;
  });

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

  it("uses the authenticated WebSocket first, ingests one frame and shows a toast", async () => {
    const sockets: TestWebSocket[] = [];
    class TestWebSocket extends EventTarget {
      static readonly OPEN = 1;
      readonly url: string;
      readonly protocols: string | string[] | undefined;
      readonly sent: string[] = [];
      readyState = TestWebSocket.OPEN;

      constructor(url: string | URL, protocols?: string | string[]) {
        super();
        this.url = String(url);
        this.protocols = protocols;
        sockets.push(this);
        queueMicrotask(() => this.dispatchEvent(new Event("open")));
      }

      send(value: string) {
        this.sent.push(value);
        const authentication = JSON.parse(value) as { type?: string; token?: string; after_id?: number };
        if (authentication.type !== "authenticate") return;
        queueMicrotask(() => {
          this.dispatchEvent(new MessageEvent("message", { data: JSON.stringify({ type: "ready", after_id: authentication.after_id }) }));
          this.dispatchEvent(new MessageEvent("message", { data: JSON.stringify({
            type: "notification", id: 4, event: "micro_event.updated",
            data: { id: 4, version: 1, monitor_id: 2, event_type: "micro_event.updated", resource_type: "micro_event", resource_id: 9, resource_version: 1, occurred_at: "2026-08-08T00:00:00Z", created_at: "2026-08-08T00:00:00Z", title: "事件更新", summary: "新增独立正文谱系", resource_status: "active", deep_link: "/dashboard/events?event=9" },
          }) }));
        });
      }

      close() {
        this.readyState = 3;
        this.dispatchEvent(new CloseEvent("close", { code: 1000, wasClean: true }));
      }
    }
    globalThis.WebSocket = TestWebSocket as unknown as typeof WebSocket;
    const fetchMock = vi.spyOn(globalThis, "fetch");

    render(<RealtimeNotifications />);

    await waitFor(() => expect(useNotificationStore.getState().items).toHaveLength(1));
    expect(sockets).toHaveLength(1);
    expect(sockets[0]?.url).toBe("ws://localhost:3000/api/v1/notifications/ws");
    expect(sockets[0]?.protocols).toEqual(["hotkey.notifications.v1"]);
    expect(JSON.parse(sockets[0]?.sent[0] ?? "{}")).toEqual({ type: "authenticate", token: "test-access-token", after_id: 0 });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(mocks.toast).toHaveBeenCalledWith("事件更新", { description: "新增独立正文谱系" });
  });
});
