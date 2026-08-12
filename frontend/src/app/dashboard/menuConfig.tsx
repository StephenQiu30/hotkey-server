import {
  BellRing,
  Database,
  Library,
  Radar,
  ShieldCheck,
  Telescope,
  Users,
} from "lucide-react";
import { UserRole } from "@/lib/domainEnums";

export interface MenuItem {
  path: string;
  name: string;
  icon: React.ReactNode;
  roles?: UserRole[];
}

export const dashboardMenuItems: MenuItem[] = [
  { path: "/dashboard", name: "概览", icon: <Telescope className="h-4 w-4" /> },
  {
    path: "/dashboard/settings",
    name: "监控",
    icon: <Radar className="h-4 w-4" />,
  },
  {
    path: "/dashboard/sources",
    name: "来源",
    icon: <Database className="h-4 w-4" />,
  },
  {
    path: "/dashboard/contents",
    name: "内容",
    icon: <Library className="h-4 w-4" />,
  },
  {
    path: "/dashboard/events",
    name: "热点事件",
    icon: <BellRing className="h-4 w-4" />,
  },
  {
    path: "/dashboard/notifications",
    name: "通知",
    icon: <BellRing className="h-4 w-4" />,
  },
];

export const dashboardAdminMenuItems: MenuItem[] = [
  {
    path: "/dashboard/users",
    name: "用户与权限",
    icon: <Users className="h-4 w-4" />,
    roles: [UserRole.Admin],
  },
  {
    path: "/dashboard/governance",
    name: "配额与审计",
    icon: <ShieldCheck className="h-4 w-4" />,
    roles: [UserRole.Admin],
  },
];
