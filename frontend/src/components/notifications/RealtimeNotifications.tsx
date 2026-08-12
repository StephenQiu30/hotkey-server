"use client";

import { useEffect } from "react";
import { toast } from "sonner";
import { AuthStatus } from "@/lib/domainEnums";
import {
  consumeNotificationStream,
  consumeNotificationWebSocket,
  openNotificationStream,
  type NotificationWebSocketFrame,
  type SSEFrame,
} from "@/lib/notificationStream";
import { getNotifications } from "@/services/hotkey/hotkey-server/notifications";
import { useAuthStore } from "@/stores/authStore";
import { useNotificationStore } from "@/stores/notificationStore";

const BACKOFF_MS = [1_000, 2_000, 4_000];
const POLLING_INTERVAL_MS = 10_000;
const SAFE_MICRO_EVENT_DEEP_LINK = /^\/dashboard\/events\?event=[1-9][0-9]{0,18}$/;

function validNotification(
  notification: HotKeyAPI.UserNotificationResponseDTO,
  id: string,
  event: string,
): HotKeyAPI.UserNotificationResponseDTO | null {
  if (
    !Number.isSafeInteger(notification.id) ||
    String(notification.id) !== id ||
    notification.event_type !== event ||
    !notification.title ||
    notification.resource_type !== "micro_event" ||
    !notification.deep_link?.match(SAFE_MICRO_EVENT_DEEP_LINK)
  ) {
    return null;
  }
  return notification;
}

function notificationFromFrame(frame: SSEFrame): HotKeyAPI.UserNotificationResponseDTO | null {
  try {
    const notification = JSON.parse(frame.data) as HotKeyAPI.UserNotificationResponseDTO;
    return validNotification(notification, frame.id, frame.event);
  } catch {
    return null;
  }
}

function notificationFromWebSocketFrame(frame: NotificationWebSocketFrame): HotKeyAPI.UserNotificationResponseDTO | null {
  return validNotification(frame.data as HotKeyAPI.UserNotificationResponseDTO, String(frame.id), frame.event);
}

function waitForRetry(milliseconds: number, signal: AbortSignal) {
  return new Promise<void>((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const timeout = window.setTimeout(done, milliseconds);
    signal.addEventListener("abort", done, { once: true });
    function done() {
      window.clearTimeout(timeout);
      signal.removeEventListener("abort", done);
      resolve();
    }
  });
}

async function pullNotifications(afterID: number) {
  const result = await getNotifications({ after_id: Math.max(0, afterID), limit: 100 });
  return result.data?.items ?? [];
}

export function RealtimeNotifications() {
  const status = useAuthStore((state) => state.status);
  const userID = useAuthStore((state) => state.user?.id);

  useEffect(() => {
    const store = useNotificationStore.getState();
    if (status !== AuthStatus.Authenticated || userID == null) {
      store.reset();
      return;
    }

    store.initializeUser(userID);
    const controller = new AbortController();
    let active = true;

    const ingestWithoutToast = async (afterID: number) => {
      const items = await pullNotifications(afterID);
      if (active) useNotificationStore.getState().ingest(items);
    };

    const run = async () => {
      const restoredCursor = useNotificationStore.getState().lastEventID;
      try {
        await ingestWithoutToast(Math.max(0, restoredCursor - 100));
      } catch {
        // The authenticated stream below remains the primary transport.
      }

      let failures = 0;
      while (active && !controller.signal.aborted) {
        const cursor = useNotificationStore.getState().lastEventID;
        useNotificationStore.getState().setTransport(failures >= 3 ? "polling" : "connecting");
        try {
          await consumeNotificationWebSocket(
            cursor,
            controller.signal,
            (frame) => ingestWithToast(notificationFromWebSocketFrame(frame)),
            () => {
              failures = 0;
              useNotificationStore.getState().setTransport("live");
            },
          );
          if (active) throw new Error("notification WebSocket ended");
        } catch {
          if (!active || controller.signal.aborted) break;
          try {
            const response = await openNotificationStream(useNotificationStore.getState().lastEventID, controller.signal);
            failures = 0;
            useNotificationStore.getState().setTransport("live");
            await consumeNotificationStream(response, (frame) => ingestWithToast(notificationFromFrame(frame)));
            if (active) throw new Error("notification SSE fallback ended");
          } catch {
            if (!active || controller.signal.aborted) break;
            failures += 1;
            if (failures >= 3) {
              useNotificationStore.getState().setTransport("polling");
              try {
                await ingestWithoutToast(useNotificationStore.getState().lastEventID);
              } catch {
                // Keep probing real-time transports without exposing internals.
              }
              await waitForRetry(POLLING_INTERVAL_MS, controller.signal);
            } else {
              await waitForRetry(BACKOFF_MS[failures - 1], controller.signal);
            }
          }
        }
      }
    };

    const ingestWithToast = (notification: HotKeyAPI.UserNotificationResponseDTO | null) => {
      if (!notification || !active) return;
      const accepted = useNotificationStore.getState().ingest([notification]);
      if (accepted.length > 0) {
        toast(notification.title ?? "收到新通知", { description: notification.summary });
      }
    };

    void run();
    return () => {
      active = false;
      controller.abort();
      useNotificationStore.getState().setTransport("idle");
    };
  }, [status, userID]);

  return null;
}
