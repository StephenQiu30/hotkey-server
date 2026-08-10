import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  browserPushSubscriptionDTO,
  createPushRegistrationIdempotencyKey,
  currentPushSubscriptionID,
  decodeVAPIDPublicKey,
  forgetCurrentPushSubscriptionID,
  getWebPushSupport,
  rememberCurrentPushSubscriptionID,
} from "@/lib/webPush";

function base64URL(bytes: Uint8Array) {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return window.btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

describe("webPush", () => {
  beforeEach(() => {
    window.localStorage.clear();
    Object.defineProperty(window, "isSecureContext", { configurable: true, value: true });
    Object.defineProperty(window, "PushManager", { configurable: true, value: class {} });
    Object.defineProperty(window, "Notification", {
      configurable: true,
      value: { permission: "default", requestPermission: vi.fn() },
    });
    Object.defineProperty(navigator, "serviceWorker", {
      configurable: true,
      value: {},
    });
  });

  it("requires a secure supported browser without requesting permission", () => {
    expect(getWebPushSupport()).toEqual({ available: true });
    expect(window.Notification.requestPermission).not.toHaveBeenCalled();

    Object.defineProperty(window, "isSecureContext", { configurable: true, value: false });
    expect(getWebPushSupport()).toMatchObject({ available: false });
  });

  it("decodes only a valid uncompressed VAPID public key", () => {
    const key = new Uint8Array(65);
    key[0] = 4;
    expect(decodeVAPIDPublicKey(base64URL(key))).toEqual(key);
    expect(() => decodeVAPIDPublicKey(base64URL(new Uint8Array(64)))).toThrow(
      "公钥无效",
    );
  });

  it("maps browser key material without placing it in local storage", () => {
    const p256dh = new Uint8Array(65);
    p256dh[0] = 4;
    const auth = new Uint8Array(16).fill(7);
    const subscription = {
      endpoint: "https://push.example/subscription",
      getKey: vi.fn((name: string) =>
        name === "p256dh" ? p256dh.buffer : auth.buffer,
      ),
    } as unknown as PushSubscription;

    expect(browserPushSubscriptionDTO(subscription)).toEqual({
      endpoint: "https://push.example/subscription",
      keys: { p256dh: base64URL(p256dh), auth: base64URL(auth) },
    });

    rememberCurrentPushSubscriptionID(9, 42);
    expect(currentPushSubscriptionID(9)).toBe(42);
    const stored = JSON.stringify(window.localStorage);
    expect(stored).not.toContain("push.example");
    expect(stored).not.toContain(base64URL(p256dh));
    expect(stored).not.toContain(base64URL(auth));
    forgetCurrentPushSubscriptionID(9);
    expect(currentPushSubscriptionID(9)).toBeNull();
  });

  it("creates a bounded non-secret idempotency key", () => {
    const spy = vi.spyOn(crypto, "randomUUID").mockReturnValue(
      "00000000-0000-4000-8000-000000000001",
    );
    expect(createPushRegistrationIdempotencyKey()).toBe(
      "push-ui:00000000-0000-4000-8000-000000000001",
    );
    spy.mockRestore();
  });
});
