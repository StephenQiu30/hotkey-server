export const HOTKEY_SERVICE_WORKER_PATH = "/sw.js";
export const WEB_PUSH_DEFAULT_TTL_SECONDS = 3600;

const CURRENT_SUBSCRIPTION_STORAGE_PREFIX =
  "hotkey.web-push.current-subscription.v1.";

type NavigatorWithStandalone = Navigator & { standalone?: boolean };

export type WebPushSupport = {
  available: boolean;
  reason?: string;
};

export type BrowserPushSubscriptionDTO = {
  endpoint: string;
  keys: {
    p256dh: string;
    auth: string;
  };
};

function currentSubscriptionStorageKey(userID: number) {
  return `${CURRENT_SUBSCRIPTION_STORAGE_PREFIX}${userID}`;
}

export function getWebPushSupport(): WebPushSupport {
  if (typeof window === "undefined" || typeof navigator === "undefined") {
    return { available: false, reason: "当前环境无法使用浏览器通知。" };
  }
  if (!window.isSecureContext) {
    return {
      available: false,
      reason: "浏览器通知需要 HTTPS 安全连接（本机 localhost 除外）。",
    };
  }
  if (
    !("serviceWorker" in navigator) ||
    !("PushManager" in window) ||
    !("Notification" in window)
  ) {
    return {
      available: false,
      reason: "当前浏览器不支持 Web Push，可继续使用站内实时通知。",
    };
  }

  const navigatorWithStandalone = navigator as NavigatorWithStandalone;
  const isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent);
  const isStandalone =
    navigatorWithStandalone.standalone === true ||
    window.matchMedia?.("(display-mode: standalone)").matches === true;
  if (isIOS && !isStandalone) {
    return {
      available: false,
      reason: "iPhone / iPad 需要先将 HotKey 添加到主屏幕，再启用通知。",
    };
  }
  return { available: true };
}

export function decodeVAPIDPublicKey(value: string): Uint8Array<ArrayBuffer> {
  const normalized = value.trim().replace(/-/g, "+").replace(/_/g, "/");
  const padding = "=".repeat((4 - (normalized.length % 4)) % 4);
  const decoded = window.atob(normalized + padding);
  const result = new Uint8Array(decoded.length);
  for (let index = 0; index < decoded.length; index += 1) {
    result[index] = decoded.charCodeAt(index);
  }
  if (result.length !== 65 || result[0] !== 4) {
    throw new Error("服务端 Web Push 公钥无效，请联系管理员。");
  }
  return result;
}

function encodeBase64URL(value: ArrayBuffer) {
  const bytes = new Uint8Array(value);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return window
    .btoa(binary)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/g, "");
}

export function browserPushSubscriptionDTO(
  subscription: PushSubscription,
): BrowserPushSubscriptionDTO {
  const p256dh = subscription.getKey("p256dh");
  const auth = subscription.getKey("auth");
  const endpoint = subscription.endpoint.trim();
  if (!endpoint.startsWith("https://") || !p256dh || !auth) {
    throw new Error("浏览器返回的通知订阅不完整，请重试。");
  }
  return {
    endpoint,
    keys: {
      p256dh: encodeBase64URL(p256dh),
      auth: encodeBase64URL(auth),
    },
  };
}

export async function registerHotKeyServiceWorker() {
  if (!("serviceWorker" in navigator)) {
    throw new Error("当前浏览器不支持 Service Worker。");
  }
  const registration = await navigator.serviceWorker.register(
    HOTKEY_SERVICE_WORKER_PATH,
    {
      scope: "/",
      updateViaCache: "none",
    },
  );
  await navigator.serviceWorker.ready;
  return registration;
}

export async function subscribeBrowserPush(
  registration: ServiceWorkerRegistration,
  vapidPublicKey: string,
) {
  const existing = await registration.pushManager.getSubscription();
  if (existing) return { subscription: existing, created: false };
  const subscription = await registration.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: decodeVAPIDPublicKey(vapidPublicKey),
  });
  return { subscription, created: true };
}

export function currentPushSubscriptionID(userID: number) {
  if (typeof window === "undefined" || userID <= 0) return null;
  try {
    const value = Number.parseInt(
      window.localStorage.getItem(currentSubscriptionStorageKey(userID)) ?? "",
      10,
    );
    return Number.isSafeInteger(value) && value > 0 ? value : null;
  } catch {
    return null;
  }
}

export function rememberCurrentPushSubscriptionID(
  userID: number,
  subscriptionID: number,
) {
  if (typeof window === "undefined" || userID <= 0 || subscriptionID <= 0) {
    return;
  }
  try {
    window.localStorage.setItem(
      currentSubscriptionStorageKey(userID),
      String(subscriptionID),
    );
  } catch {
    // Private browsing or storage policy may reject localStorage. The server
    // subscription remains valid; the user can still manage it in the list.
  }
}

export function forgetCurrentPushSubscriptionID(userID: number) {
  if (typeof window === "undefined" || userID <= 0) return;
  try {
    window.localStorage.removeItem(currentSubscriptionStorageKey(userID));
  } catch {
    // Nothing else should be persisted as a fallback.
  }
}

export function createPushRegistrationIdempotencyKey() {
  if (typeof crypto === "undefined" || !crypto.randomUUID) {
    throw new Error("当前浏览器无法安全生成订阅请求标识。");
  }
  return `push-ui:${crypto.randomUUID()}`;
}

export function browserDeviceLabel() {
  if (typeof navigator === "undefined") return "此浏览器";
  const userAgent = navigator.userAgent;
  if (/iPad/.test(userAgent)) return "iPad";
  if (/iPhone|iPod/.test(userAgent)) return "iPhone";
  if (/Android/.test(userAgent)) return "Android 设备";
  if (/Macintosh|Mac OS X/.test(userAgent)) return "Mac 浏览器";
  if (/Windows/.test(userAgent)) return "Windows 浏览器";
  if (/Linux/.test(userAgent)) return "Linux 浏览器";
  return "此浏览器";
}

export function browserTimezone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}
