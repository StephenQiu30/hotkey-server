import { describe, expect, it } from "vitest";
import {
  dashboardAdminMenuItems,
  dashboardMenuItems,
} from "@/app/dashboard/menuConfig";
import { UserRole } from "@/lib/domainEnums";

describe("dashboard menu", () => {
  it("keeps the primary navigation free of the duplicate notification entry", () => {
    expect(dashboardMenuItems.map(({ path, name }) => ({ path, name }))).toEqual([
      { path: "/dashboard", name: "概览" },
      { path: "/dashboard/settings", name: "监控" },
      { path: "/dashboard/sources", name: "来源" },
      { path: "/dashboard/contents", name: "内容" },
      { path: "/dashboard/events", name: "事件" },
      { path: "/dashboard/alerts", name: "告警" },
      { path: "/dashboard/reports", name: "报告" },
      { path: "/dashboard/subscriptions", name: "订阅" },
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
