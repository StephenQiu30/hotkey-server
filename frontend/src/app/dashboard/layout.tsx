"use client";

import AuthGuard from "@/components/AuthGuard";
import BasicLayout from "@/layouts/BasicLayout";
import { dashboardAdminMenuItems, dashboardMenuItems } from "./menuConfig";

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  return (
    <AuthGuard>
      <BasicLayout
        menuItems={dashboardMenuItems}
        adminMenuItems={dashboardAdminMenuItems}
        title="HotKey"
      >
        {children}
      </BasicLayout>
    </AuthGuard>
  );
}
