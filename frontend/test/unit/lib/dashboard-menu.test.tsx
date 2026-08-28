import { describe, expect, it } from "vitest";
import {
  dashboardAdminMenuItems,
  dashboardMenuItems,
} from "@/app/dashboard/menuConfig";
import { UserRole } from "@/lib/domainEnums";

describe("dashboard menu", () => {
  it("exposes the hotspot-to-semantic-event workflow in primary navigation", () => {
    expect(
      dashboardMenuItems.map(({ path, name }) => ({ path, name }))
    ).toEqual([
      { path: "/dashboard", name: "概览" },
      { path: "/dashboard/settings", name: "监控" },
      { path: "/dashboard/sources", name: "来源" },
      { path: "/dashboard/search", name: "全文检索" },
      { path: "/dashboard/reports", name: "日报" },
      { path: "/dashboard/knowledge", name: "知识投影" },
      { path: "/dashboard/contents", name: "热点雷达" },
      { path: "/dashboard/events", name: "语义事件" },
    ]);
  });

  it("keeps only administrator-only governance in the protected menu", () => {
    expect(
      dashboardAdminMenuItems.map(({ path, name }) => ({ path, name }))
    ).toEqual([
      { path: "/dashboard/users", name: "用户与权限" },
      { path: "/dashboard/governance", name: "配额与审计" },
    ]);
    expect(
      dashboardAdminMenuItems.every((item) =>
        item.roles?.includes(UserRole.Admin)
      )
    ).toBe(true);
  });

  it("exposes the safe source directory to every product role", () => {
    expect(
      dashboardMenuItems.find((item) => item.path === "/dashboard/sources")
        ?.roles
    ).toBeUndefined();
  });

  it("keeps knowledge publishing inside the editor and administrator boundary", () => {
    expect(
      dashboardMenuItems.find((item) => item.path === "/dashboard/knowledge")
        ?.roles,
    ).toEqual([UserRole.Editor, UserRole.Admin]);
  });
});
