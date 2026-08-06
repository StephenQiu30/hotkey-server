import {
  CollectionRunStatus,
  DeliveryChannel,
  MonitorStatus,
  ReportType,
  SourceHealthStatus,
} from "./domainEnums";

export interface StatusPresentation {
  label: string;
  className: string;
}

const sourceHealthPresentations: Record<
  SourceHealthStatus,
  StatusPresentation
> = {
  [SourceHealthStatus.Healthy]: {
    label: "健康",
    className: "text-green-500",
  },
  [SourceHealthStatus.Degraded]: {
    label: "降级",
    className: "text-amber-400",
  },
  [SourceHealthStatus.Unavailable]: {
    label: "不可用",
    className: "text-red-400",
  },
  [SourceHealthStatus.Unknown]: {
    label: "未探测",
    className: "text-muted-foreground",
  },
};

const monitorStatusLabels: Record<MonitorStatus, string> = {
  [MonitorStatus.Draft]: "草稿",
  [MonitorStatus.Active]: "运行中",
  [MonitorStatus.Paused]: "已暂停",
  [MonitorStatus.Archived]: "已归档",
};

const reportTypeLabels: Record<ReportType, string> = {
  [ReportType.Daily]: "日报",
  [ReportType.Weekly]: "周报",
};

const deliveryChannelLabels: Record<DeliveryChannel, string> = {
  [DeliveryChannel.Email]: "邮件",
  [DeliveryChannel.RSS]: "RSS",
};

const collectionRunPresentations: Record<
  CollectionRunStatus,
  StatusPresentation
> = {
  [CollectionRunStatus.Queued]: {
    label: "排队中",
    className: "text-muted-foreground",
  },
  [CollectionRunStatus.Running]: {
    label: "采集中",
    className: "text-blue-400",
  },
  [CollectionRunStatus.Succeeded]: {
    label: "成功",
    className: "text-green-500",
  },
  [CollectionRunStatus.Failed]: {
    label: "失败",
    className: "text-red-400",
  },
  [CollectionRunStatus.Cancelled]: {
    label: "已取消",
    className: "text-muted-foreground",
  },
};

export function sourceHealthPresentation(
  status: string | undefined,
): StatusPresentation {
  if (Object.values(SourceHealthStatus).includes(status as SourceHealthStatus)) {
    return sourceHealthPresentations[status as SourceHealthStatus];
  }
  return sourceHealthPresentations[SourceHealthStatus.Unknown];
}

export function monitorStatusLabel(status: string | undefined): string {
  return monitorStatusLabels[status as MonitorStatus] ?? status ?? "—";
}

export function reportTypeLabel(type: string | undefined): string {
  return reportTypeLabels[type as ReportType] ?? type ?? "报告";
}

export function deliveryChannelLabel(channel: string | undefined): string {
  return deliveryChannelLabels[channel as DeliveryChannel] ?? channel ?? "—";
}

export function collectionRunPresentation(
  status: string | undefined,
): StatusPresentation {
  return (
    collectionRunPresentations[status as CollectionRunStatus] ?? {
      label: status ?? "未知",
      className: "text-muted-foreground",
    }
  );
}
