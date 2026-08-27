import {
  Activity,
  Database,
  Flame,
  Radar,
  Search,
  ShieldCheck,
  Telescope,
  Users,
} from "lucide-react";
import { UserRole } from "@/lib/domainEnums";
import { dashboardRouteRoles } from "@/lib/dashboardAccess";

export interface MenuItem {
  path: string;
  name: string;
  icon: React.ReactNode;
  roles?: readonly UserRole[];
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
    path: "/dashboard/search",
    name: "即时搜索",
    icon: <Search className="h-4 w-4" />,
  },
  {
    path: "/dashboard/contents",
    name: "热点雷达",
    icon: <Flame className="h-4 w-4" />,
  },
  {
    path: "/dashboard/events",
    name: "语义事件",
    icon: <Activity className="h-4 w-4" />,
  },
];

export const dashboardAdminMenuItems: MenuItem[] = [
  {
    path: "/dashboard/users",
    name: "用户与权限",
    icon: <Users className="h-4 w-4" />,
    roles: dashboardRouteRoles["/dashboard/users"],
  },
  {
    path: "/dashboard/governance",
    name: "配额与审计",
    icon: <ShieldCheck className="h-4 w-4" />,
    roles: dashboardRouteRoles["/dashboard/governance"],
  },
];
