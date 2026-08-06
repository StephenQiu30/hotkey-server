"use client";

import { useEffect, useState } from "react";
import TopNav from "@/components/dashboard/TopNav";
import { getAlerts } from "@/services/hotkey/hotkey-server/alerts";

interface MenuItem {
  path: string;
  name: string;
  icon: React.ReactNode;
}

export default function BasicLayout({
  children,
  menuItems,
  adminMenuItems = [],
  title = "HotKey",
}: {
  children: React.ReactNode;
  menuItems: MenuItem[];
  adminMenuItems?: MenuItem[];
  title?: string;
}) {
  const [alertCount, setAlertCount] = useState(0);

  useEffect(() => {
    let active = true;
    getAlerts({ state: "open", limit: 100 })
      .then((result) => {
        if (active) setAlertCount(result.data?.items?.length ?? 0);
      })
      .catch(() => {
        if (active) setAlertCount(0);
      });
    return () => {
      active = false;
    };
  }, []);

  return (
    <div className="min-h-screen bg-background text-foreground">
      <TopNav
        menuItems={menuItems}
        adminMenuItems={adminMenuItems}
        title={title}
        alertCount={alertCount}
      />
      <main>{children}</main>
    </div>
  );
}
