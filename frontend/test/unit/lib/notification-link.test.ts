import { describe, expect, it } from "vitest";
import { isSafeNotificationDeepLink } from "@/lib/notificationLink";

describe("isSafeNotificationDeepLink", () => {
  it("keeps each notification resource on its own safe dashboard route", () => {
    expect(isSafeNotificationDeepLink("hotspot", "/dashboard/contents/9")).toBe(true);
    expect(isSafeNotificationDeepLink("micro_event", "/dashboard/events?event=9")).toBe(true);
    expect(isSafeNotificationDeepLink("hotspot", "/dashboard/events?event=9")).toBe(false);
    expect(isSafeNotificationDeepLink("micro_event", "/dashboard/contents/9")).toBe(false);
    expect(isSafeNotificationDeepLink("hotspot", "https://evil.test")).toBe(false);
  });
});
