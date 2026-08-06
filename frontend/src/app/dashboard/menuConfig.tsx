import {
  BellRing,
  Database,
  FileText,
  Library,
  Radar,
  Send,
  Telescope,
} from "lucide-react";

export interface MenuItem { path: string; name: string; icon: React.ReactNode; }

export const dashboardMenuItems: MenuItem[] = [
  { path: "/dashboard", name: "概览", icon: <Telescope className="h-4 w-4" /> },
  { path: "/dashboard/settings", name: "监控", icon: <Radar className="h-4 w-4" /> },
  { path: "/dashboard/events", name: "事件", icon: <BellRing className="h-4 w-4" /> },
  { path: "/dashboard/reports", name: "简报", icon: <FileText className="h-4 w-4" /> },
];

export const dashboardAdminMenuItems: MenuItem[] = [
  { path: "/dashboard/contents", name: "采集内容", icon: <Library className="h-4 w-4" /> },
  { path: "/dashboard/notifications", name: "发布订阅", icon: <Send className="h-4 w-4" /> },
  { path: "/dashboard/sources", name: "来源管理", icon: <Database className="h-4 w-4" /> },
];
