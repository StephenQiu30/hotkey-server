const RESOURCE_DEEP_LINKS: Readonly<Record<string, RegExp>> = {
  hotspot: /^\/dashboard\/contents\/[1-9][0-9]{0,18}$/,
  micro_event: /^\/dashboard\/events\?event=[1-9][0-9]{0,18}$/,
};

export function isSafeNotificationDeepLink(
  resourceType: string | undefined,
  deepLink: string | undefined,
) {
  if (!resourceType || !deepLink) return false;
  return RESOURCE_DEEP_LINKS[resourceType]?.test(deepLink) ?? false;
}
