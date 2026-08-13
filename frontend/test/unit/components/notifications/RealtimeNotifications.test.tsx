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
            type: "notification", id: 4, event: "hotspot.discovered",
            data: { id: 4, version: 1, monitor_id: 2, event_type: "hotspot.discovered", resource_type: "hotspot", resource_id: 9, resource_version: 1, occurred_at: "2026-08-08T00:00:00Z", created_at: "2026-08-08T00:00:00Z", title: "新热点", summary: "监控词命中", resource_status: "high", deep_link: "/dashboard/contents/9" },
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
    expect(mocks.toast).toHaveBeenCalledWith("新热点", { description: "监控词命中" });
  });

  it("pulls the durable REST cursor when WebSocket is temporarily unavailable", async () => {
    class FailingWebSocket extends EventTarget {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      readyState = FailingWebSocket.CONNECTING;

      constructor() {
        super();
        queueMicrotask(() => this.dispatchEvent(new Event("error")));
      }

      send() {}
      close() { this.readyState = 3; }
    }
    globalThis.WebSocket = FailingWebSocket as unknown as typeof WebSocket;
    mocks.getNotifications
      .mockResolvedValueOnce({ data: { items: [], next_after_id: 0 } })
      .mockResolvedValue({
        data: {
          next_after_id: 5,
          items: [{
            id: 5, version: 1, monitor_id: 2, event_type: "hotspot.discovered",
            resource_type: "hotspot", resource_id: 10, resource_version: 1,
            occurred_at: "2026-08-08T00:00:00Z", created_at: "2026-08-08T00:00:00Z",
            title: "补拉热点", summary: "来自持久游标", resource_status: "high",
            deep_link: "/dashboard/contents/10",
          }],
        },
      });

    render(<RealtimeNotifications />);

    await waitFor(() => expect(mocks.getNotifications).toHaveBeenCalledTimes(2));
    expect(mocks.getNotifications).toHaveBeenLastCalledWith({ after_id: 0, limit: 100 });
    expect(useNotificationStore.getState().transport).toBe("polling");
    expect(useNotificationStore.getState().items[0]?.title).toBe("补拉热点");
    expect(mocks.toast).not.toHaveBeenCalled();
  });
});
