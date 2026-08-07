import TopNav from "@/components/dashboard/TopNav";
import { RealtimeNotifications } from "@/components/notifications/RealtimeNotifications";

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
  return (
    <div className="min-h-screen bg-background text-foreground">
      <RealtimeNotifications />
      <TopNav
        menuItems={menuItems}
        adminMenuItems={adminMenuItems}
        title={title}
      />
      <main className="min-w-0">{children}</main>
    </div>
  );
}
