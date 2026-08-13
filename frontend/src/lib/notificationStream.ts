import { getAccessToken } from "@/lib/authSession";

export interface NotificationWebSocketFrame {
  type: "notification";
  id: number;
  event: string;
  data: unknown;
}

const NOTIFICATION_WEBSOCKET_PROTOCOL = "hotkey.notifications.v1";
const MAX_NOTIFICATION_WEBSOCKET_FRAME_BYTES = 65_536;

export function notificationWebSocketURL(location: Location = window.location): string {
  const url = new URL("/api/v1/notifications/ws", location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

/**
 * Opens the same-origin notification WebSocket and authenticates in the first
 * application frame. The short-lived token is never placed in the URL or in a
 * negotiated subprotocol value.
 */
export function consumeNotificationWebSocket(
  afterID: number,
  signal: AbortSignal,
  onFrame: (frame: NotificationWebSocketFrame) => void,
  onReady?: (afterID: number) => void,
): Promise<void> {
  const token = getAccessToken();
  if (!token) return Promise.reject(new Error("notification WebSocket access token is unavailable"));
  if (signal.aborted) return Promise.resolve();

  const cursor = Math.max(0, Math.trunc(afterID));
  return new Promise<void>((resolve, reject) => {
    const socket = new WebSocket(notificationWebSocketURL(), [NOTIFICATION_WEBSOCKET_PROTOCOL]);
    let authenticated = false;
    let settled = false;

    const finish = (error?: Error) => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", abort);
      if (error) reject(error);
      else resolve();
    };
    const rejectProtocol = (message: string) => {
      try {
        socket.close(1002, "invalid notification frame");
      } finally {
        finish(new Error(message));
      }
    };
    const abort = () => {
      if (socket.readyState === WebSocket.CONNECTING || socket.readyState === WebSocket.OPEN) {
        socket.close(1000, "client stopped");
      }
      finish();
    };

    signal.addEventListener("abort", abort, { once: true });
    socket.addEventListener("open", () => {
      socket.send(JSON.stringify({ type: "authenticate", token, after_id: cursor }));
    });
    socket.addEventListener("message", (event) => {
      if (typeof event.data !== "string" || new TextEncoder().encode(event.data).byteLength > MAX_NOTIFICATION_WEBSOCKET_FRAME_BYTES) {
        rejectProtocol("notification WebSocket frame is invalid");
        return;
      }
      let frame: Record<string, unknown>;
      try {
        frame = JSON.parse(event.data) as Record<string, unknown>;
      } catch {
        rejectProtocol("notification WebSocket frame is not JSON");
        return;
      }
      if (frame.type === "ready") {
        if (authenticated || !Number.isSafeInteger(frame.after_id) || Number(frame.after_id) < cursor) {
          rejectProtocol("notification WebSocket ready frame is invalid");
          return;
        }
        authenticated = true;
        onReady?.(Number(frame.after_id));
        return;
      }
      if (frame.type === "heartbeat") {
        if (!authenticated || !Number.isSafeInteger(frame.after_id)) rejectProtocol("notification WebSocket heartbeat is invalid");
        return;
      }
      if (frame.type !== "notification" || !authenticated || !Number.isSafeInteger(frame.id) || typeof frame.event !== "string" || typeof frame.data !== "object" || frame.data == null) {
        rejectProtocol("notification WebSocket business frame is invalid");
        return;
      }
      onFrame(frame as unknown as NotificationWebSocketFrame);
    });
    socket.addEventListener("error", () => finish(new Error("notification WebSocket failed")));
    socket.addEventListener("close", () => {
      if (signal.aborted) finish();
      else finish(new Error("notification WebSocket ended"));
    });
  });
}
