import {
  Activity,
  Database,
  FileText,
  Flame,
  LibraryBig,
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
  { path: "/dashboard", name: "今日态势", icon: <Telescope className="h-4 w-4" /> },
  {
    path: "/dashboard/settings",
    name: "监控任务",
    icon: <Radar className="h-4 w-4" />,
  },
  {
    path: "/dashboard/sources",
    name: "来源与覆盖",
    icon: <Database className="h-4 w-4" />,
  },
  {
    path: "/dashboard/search",
    name: "情报检索",
    icon: <Search className="h-4 w-4" />,
  },
  {
    path: "/dashboard/reports",
    name: "情报简报",
    icon: <FileText className="h-4 w-4" />,
  },
  {
    path: "/dashboard/knowledge",
    name: "知识库",
    icon: <LibraryBig className="h-4 w-4" />,
    roles: dashboardRouteRoles["/dashboard/knowledge"],
  },
  {
    path: "/dashboard/contents",
    name: "信号流",
    icon: <Flame className="h-4 w-4" />,
  },
  {
    path: "/dashboard/events",
    name: "事件雷达",
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
    name: "系统治理",
    icon: <ShieldCheck className="h-4 w-4" />,
    roles: dashboardRouteRoles["/dashboard/governance"],
  },
];
