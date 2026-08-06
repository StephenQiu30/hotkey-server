import { describe, expect, it } from "vitest";
import {
  APIErrorCode,
  AuthStatus,
  DeliveryChannel,
  MonitorAction,
  MonitorStatus,
  ReportAction,
  ReportStatus,
  ReportType,
  SourceHealthDiagnostic,
  SourceHealthStatus,
  SourceType,
  UserRole,
} from "@/lib/domainEnums";
import {
  deliveryChannelLabel,
  monitorStatusLabel,
  reportTypeLabel,
  sourceHealthPresentation,
} from "@/lib/domainPresentation";

describe("domain enums", () => {
  it("preserves the backend wire values", () => {
    expect(UserRole.Admin).toBe("admin");
    expect(AuthStatus.Authenticated).toBe("authenticated");
    expect(SourceType.HackerNews).toBe("hacker_news");
    expect(SourceHealthStatus.Unavailable).toBe("unavailable");
    expect(MonitorStatus.Paused).toBe("paused");
    expect(MonitorAction.Publish).toBe("publish");
    expect(ReportStatus.Published).toBe("published");
    expect(ReportAction.Preview).toBe("preview");
    expect(ReportType.Weekly).toBe("weekly");
    expect(DeliveryChannel.RSS).toBe("rss");
    expect(APIErrorCode.Forbidden).toBe(20001);
    expect(SourceHealthDiagnostic.DestinationNotPermitted).toBe(
      "destination_not_permitted",
    );
  });

  it("provides centralized user-facing presentations", () => {
    expect(sourceHealthPresentation(SourceHealthStatus.Healthy)).toEqual({
      label: "健康",
      className: "text-green-500",
    });
    expect(sourceHealthPresentation("future_status")).toEqual({
      label: "未探测",
      className: "text-muted-foreground",
    });
    expect(monitorStatusLabel(MonitorStatus.Active)).toBe("运行中");
    expect(reportTypeLabel(ReportType.Daily)).toBe("日报");
    expect(deliveryChannelLabel(DeliveryChannel.Email)).toBe("邮件");
  });
});
