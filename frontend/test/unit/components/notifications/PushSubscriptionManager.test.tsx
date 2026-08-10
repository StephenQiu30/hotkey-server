import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PushSubscriptionManager } from "@/components/notifications/PushSubscriptionManager";
import { AuthStatus } from "@/lib/domainEnums";
import { getMonitors } from "@/services/hotkey/hotkey-server/monitors";
import {
  getNotificationsPushCapability,
  getNotificationsPushSubscriptions,
  postNotificationsPushSubscriptions,
} from "@/services/hotkey/hotkey-server/notifications";
import { useAuthStore } from "@/stores/authStore";

vi.mock("@/services/hotkey/hotkey-server/monitors", () => ({ getMonitors: vi.fn() }));
vi.mock("@/services/hotkey/hotkey-server/notifications", () => ({
  getNotificationsPushCapability: vi.fn(),
  getNotificationsPushSubscriptions: vi.fn(),
  postNotificationsPushSubscriptions: vi.fn(),
  putNotificationsPushSubscriptionsId: vi.fn(),
  deleteNotificationsPushSubscriptionsId: vi.fn(),
}));

function base64URL(bytes: Uint8Array) {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return window.btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

describe("PushSubscriptionManager", () => {
  const p256dh = new Uint8Array(65);
  const auth = new Uint8Array(16).fill(9);
  const unsubscribe = vi.fn(() => Promise.resolve(true));
  const browserSubscription = {
    endpoint: "https://push.example/subscription",
    getKey: vi.fn((name: string) => (name === "p256dh" ? p256dh.buffer : auth.buffer)),
    unsubscribe,
  } as unknown as PushSubscription;
  const subscribe = vi.fn(() => Promise.resolve(browserSubscription));
  const registration = {
    pushManager: {
      getSubscription: vi.fn(() => Promise.resolve(null)),
      subscribe,
    },
  } as unknown as ServiceWorkerRegistration;
  const requestPermission = vi.fn<() => Promise<NotificationPermission>>();

  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    p256dh.fill(0);
    p256dh[0] = 4;
    Object.defineProperty(window, "isSecureContext", { configurable: true, value: true });
    Object.defineProperty(window, "PushManager", { configurable: true, value: class {} });
    Object.defineProperty(window, "Notification", {
      configurable: true,
      value: { permission: "default", requestPermission },
    });
    Object.defineProperty(navigator, "serviceWorker", {
      configurable: true,
      value: {
        getRegistration: vi.fn(() => Promise.resolve(undefined)),
        register: vi.fn(() => Promise.resolve(registration)),
        ready: Promise.resolve(registration),
      },
    });
    vi.spyOn(crypto, "randomUUID").mockReturnValue(
      "00000000-0000-4000-8000-000000000002",
    );
    useAuthStore.setState({
      status: AuthStatus.Authenticated,
      user: {
        id: 7,
        email: "user@example.com",
        display_name: "User",
        role: "editor",
        status: "active",
      },
      error: null,
    });
    vi.mocked(getNotificationsPushCapability).mockResolvedValue({
      code: 0,
      data: { available: true, vapid_public_key: base64URL(p256dh) },
      message: "ok",
    });
    vi.mocked(getNotificationsPushSubscriptions).mockResolvedValue({
      code: 0,
      data: { items: [] },
      message: "ok",
    });
    vi.mocked(getMonitors).mockResolvedValue({
      code: 0,
      data: { items: [{ id: 12, name: "AI 芯片", status: "active", version: 1 }] },
      message: "ok",
    });
  });

  it("does not request permission until an explicit opt-in gesture", async () => {
    render(<PushSubscriptionManager />);
    expect(await screen.findByText("在此设备启用通知")).toBeInTheDocument();
    expect(requestPermission).not.toHaveBeenCalled();
    expect(postNotificationsPushSubscriptions).not.toHaveBeenCalled();
  });

  it("subscribes one explicitly selected monitor and stores no browser secret", async () => {
    requestPermission.mockResolvedValue("granted");
    vi.mocked(postNotificationsPushSubscriptions).mockResolvedValue({
      code: 0,
      message: "ok",
      data: {
        id: 31,
        version: 1,
        device_label: "Linux 浏览器",
        timezone: "UTC",
        ttl_seconds: 3600,
        status: "active",
        monitor_ids: [12],
        created_at: "2026-08-10T00:00:00Z",
        updated_at: "2026-08-10T00:00:00Z",
      },
    });
    const user = userEvent.setup();
    render(<PushSubscriptionManager />);
    await screen.findByText("在此设备启用通知");
    await user.click(screen.getByText("AI 芯片"));
    await user.click(screen.getByRole("button", { name: "启用此设备通知" }));

    await waitFor(() => expect(postNotificationsPushSubscriptions).toHaveBeenCalledTimes(1));
    expect(requestPermission).toHaveBeenCalledTimes(1);
    expect(subscribe).toHaveBeenCalledWith(
      expect.objectContaining({ userVisibleOnly: true, applicationServerKey: expect.any(Uint8Array) }),
    );
    expect(postNotificationsPushSubscriptions).toHaveBeenCalledWith(
      expect.objectContaining({
        endpoint: "https://push.example/subscription",
        keys: { p256dh: base64URL(p256dh), auth: base64URL(auth) },
        monitor_ids: [12],
        ttl_seconds: 3600,
      }),
      {
        headers: {
          "Idempotency-Key": "push-ui:00000000-0000-4000-8000-000000000002",
        },
      },
    );
    expect(await screen.findByText("当前浏览器")).toBeInTheDocument();
    const stored = JSON.stringify(window.localStorage);
    expect(stored).toContain("31");
    expect(stored).not.toContain("push.example");
    expect(stored).not.toContain(base64URL(p256dh));
    expect(stored).not.toContain(base64URL(auth));
  });

  it("surfaces denied permission and never creates a server subscription", async () => {
    requestPermission.mockResolvedValue("denied");
    const user = userEvent.setup();
    render(<PushSubscriptionManager />);
    await screen.findByText("在此设备启用通知");
    await user.click(screen.getByText("AI 芯片"));
    await user.click(screen.getByRole("button", { name: "启用此设备通知" }));
    expect(await screen.findByText("浏览器通知权限已被拒绝")).toBeInTheDocument();
    expect(postNotificationsPushSubscriptions).not.toHaveBeenCalled();
  });

  it("degrades to the inbox without a secure context", async () => {
    Object.defineProperty(window, "isSecureContext", { configurable: true, value: false });
    render(<PushSubscriptionManager />);
    expect(await screen.findByText("当前设备暂不支持推送")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "启用此设备通知" })).not.toBeInTheDocument();
    expect(requestPermission).not.toHaveBeenCalled();
  });
});
