import { beforeEach, describe, expect, it } from "vitest";
import { useNotificationStore } from "@/stores/notificationStore";

const notification = (id: number): HotKeyAPI.UserNotificationResponseDTO => ({
  id,
  version: 1,
  monitor_id: 2,
  event_type: "micro_event.updated",
  resource_type: "micro_event",
  resource_id: id,
  resource_version: 1,
  occurred_at: "2026-08-08T00:00:00Z",
  created_at: "2026-08-08T00:00:00Z",
  title: `通知 ${id}`,
  resource_status: "active",
  deep_link: `/dashboard/events?event=${id}`,
});

describe("notificationStore", () => {
  beforeEach(() => {
    localStorage.clear();
    useNotificationStore.getState().reset();
  });

  it("deduplicates, bounds memory and persists cursor IDs only", () => {
    useNotificationStore.getState().initializeUser(7);
    const accepted = useNotificationStore
      .getState()
      .ingest([...Array.from({ length: 105 }, (_, index) => notification(index + 1)), notification(105)]);

    const state = useNotificationStore.getState();
    expect(accepted).toHaveLength(105);
    expect(state.items).toHaveLength(100);
    expect(state.items[0].id).toBe(105);
    expect(state.lastEventID).toBe(105);
    expect(state.unreadCount).toBe(100);
    expect(localStorage.getItem("hotkey.notifications.v2.7")).toBe(
      JSON.stringify({ lastEventID: 105, readThroughID: 0 }),
    );
    expect(localStorage.getItem("hotkey.notifications.v2.7")).not.toContain("通知");
  });

  it("restores per-user cursors and marks all loaded notifications read", () => {
    localStorage.setItem(
      "hotkey.notifications.v2.9",
      JSON.stringify({ lastEventID: 8, readThroughID: 5 }),
    );
    useNotificationStore.getState().initializeUser(9);
    useNotificationStore.getState().ingest([notification(6), notification(8), notification(10)]);
    expect(useNotificationStore.getState().unreadCount).toBe(3);

    useNotificationStore.getState().markAllRead();
    expect(useNotificationStore.getState().unreadCount).toBe(0);
    expect(useNotificationStore.getState().readThroughID).toBe(10);
  });
});
