import TopNav from "@/components/dashboard/TopNav";
import { RealtimeNotifications } from "@/components/notifications/RealtimeNotifications";
import { SkipLink } from "@/components/SkipLink";

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
      <SkipLink />
      <RealtimeNotifications />
      <TopNav
        menuItems={menuItems}
        adminMenuItems={adminMenuItems}
        title={title}
      />
      <main className="min-w-0" id="main-content" tabIndex={-1}>
        {children}
      </main>
    </div>
  );
}
