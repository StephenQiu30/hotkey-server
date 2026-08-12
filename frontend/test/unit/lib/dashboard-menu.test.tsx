import { describe, expect, it } from "vitest";
import {
  dashboardAdminMenuItems,
  dashboardMenuItems,
} from "@/app/dashboard/menuConfig";
import { UserRole } from "@/lib/domainEnums";

describe("dashboard menu", () => {
  it("keeps only the X hotspot core workflow in primary navigation", () => {
    expect(dashboardMenuItems.map(({ path, name }) => ({ path, name }))).toEqual([
      { path: "/dashboard", name: "概览" },
      { path: "/dashboard/settings", name: "监控" },
      { path: "/dashboard/sources", name: "来源" },
      { path: "/dashboard/contents", name: "内容" },
      { path: "/dashboard/events", name: "热点事件" },
      { path: "/dashboard/notifications", name: "通知" },
    ]);
  });

  it("keeps only administrator-only governance in the protected menu", () => {
    expect(dashboardAdminMenuItems.map(({ path, name }) => ({ path, name }))).toEqual([
      { path: "/dashboard/users", name: "用户与权限" },
      { path: "/dashboard/governance", name: "配额与审计" },
    ]);
    expect(
      dashboardAdminMenuItems.every((item) =>
        item.roles?.includes(UserRole.Admin)
      )
    ).toBe(true);
  });
});
