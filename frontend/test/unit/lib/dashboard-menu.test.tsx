import { describe, expect, it } from "vitest";
import {
  dashboardAdminMenuItems,
  dashboardMenuItems,
} from "@/app/dashboard/menuConfig";

describe("dashboard menu", () => {
  it("uses the selected four-part SaaS information architecture", () => {
    expect(dashboardMenuItems.map(({ path, name }) => ({ path, name }))).toEqual([
      { path: "/dashboard", name: "概览" },
      { path: "/dashboard/settings", name: "监控" },
      { path: "/dashboard/events", name: "事件" },
      { path: "/dashboard/reports", name: "简报" },
    ]);
  });

  it("keeps data and delivery operations out of the primary navigation", () => {
    expect(dashboardAdminMenuItems.map(({ path, name }) => ({ path, name }))).toEqual([
      { path: "/dashboard/contents", name: "采集内容" },
      { path: "/dashboard/notifications", name: "发布订阅" },
      { path: "/dashboard/sources", name: "来源管理" },
    ]);
  });
});
