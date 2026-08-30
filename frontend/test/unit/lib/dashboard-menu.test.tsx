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
      { path: "/dashboard", name: "今日态势" },
      { path: "/dashboard/settings", name: "监控任务" },
      { path: "/dashboard/sources", name: "来源与覆盖" },
      { path: "/dashboard/search", name: "情报检索" },
      { path: "/dashboard/reports", name: "情报简报" },
      { path: "/dashboard/knowledge", name: "知识库" },
      { path: "/dashboard/contents", name: "信号流" },
      { path: "/dashboard/events", name: "事件雷达" },
    ]);
  });

  it("keeps only administrator-only governance in the protected menu", () => {
    expect(
      dashboardAdminMenuItems.map(({ path, name }) => ({ path, name }))
    ).toEqual([
      { path: "/dashboard/users", name: "用户与权限" },
      { path: "/dashboard/governance", name: "系统治理" },
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
