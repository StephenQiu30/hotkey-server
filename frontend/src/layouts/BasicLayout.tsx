import TopNav from "@/components/dashboard/TopNav";
import { RealtimeNotifications } from "@/components/notifications/RealtimeNotifications";
import { BrandLogo } from "@/components/BrandLogo";
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
    <div
      className="flex h-dvh min-h-screen flex-col overflow-hidden bg-background text-foreground"
      data-testid="basic-layout"
    >
      <SkipLink />
      <RealtimeNotifications />
      <TopNav
        menuItems={menuItems}
        adminMenuItems={adminMenuItems}
        title={title}
      />
      <main
        className="app-main min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain"
        data-layout-scroll-region
        id="main-content"
        tabIndex={-1}
      >
        {children}
      </main>
      <footer
        aria-label="工作台页脚"
        className="shrink-0 border-t border-border/70 bg-background/95"
        data-layout-footer
      >
        <div className="app-shell-container flex h-10 items-center justify-between gap-4 text-xs text-muted-foreground">
          <BrandLogo
            className="shrink-0 font-medium text-foreground"
            markClassName="h-3.5 w-3.5"
          />
          <span className="min-w-0 truncate">© 2026 HotKey · 让热点判断建立在证据之上</span>
        </div>
      </footer>
    </div>
  );
}
