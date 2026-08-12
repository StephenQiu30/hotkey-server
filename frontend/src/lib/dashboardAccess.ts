import { UserRole } from "@/lib/domainEnums";

export const dashboardRouteRoles = {
  "/dashboard/sources": [UserRole.Admin],
  "/dashboard/users": [UserRole.Admin],
  "/dashboard/governance": [UserRole.Admin],
} as const satisfies Record<string, readonly UserRole[]>;

export function canAccessRoles(
  roles: readonly UserRole[] | undefined,
  role: string | undefined,
) {
  return !roles || Boolean(role && roles.includes(role as UserRole));
}

export function getDashboardRouteRoles(pathname: string) {
  const policy = Object.entries(dashboardRouteRoles).find(
    ([path]) => pathname === path || pathname.startsWith(`${path}/`),
  );
  return policy?.[1] as readonly UserRole[] | undefined;
}

export function canAccessDashboardRoute(
  pathname: string,
  role: string | undefined,
) {
  return canAccessRoles(getDashboardRouteRoles(pathname), role);
}
