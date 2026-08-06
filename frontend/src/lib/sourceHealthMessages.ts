import { SourceHealthDiagnostic } from "./domainEnums";

const SOURCE_HEALTH_MESSAGES: Readonly<Record<SourceHealthDiagnostic, string>> = {
  [SourceHealthDiagnostic.InvalidSourceConnection]: "来源配置无效，请检查地址与类型",
  [SourceHealthDiagnostic.RequestFailed]: "无法连接来源，请检查网络后重试",
  [SourceHealthDiagnostic.UpstreamStatus]: "来源服务返回异常状态，请稍后重试",
  [SourceHealthDiagnostic.ConnectorUnavailable]: "当前来源类型暂不可用",
  [SourceHealthDiagnostic.DestinationNotPermitted]: "来源地址被安全策略拒绝，请检查 DNS 或代理的 Fake-IP 设置",
};

export function getSourceHealthMessage(code: string | undefined): string {
  if (!code) return "来源暂不可用";
  return SOURCE_HEALTH_MESSAGES[code as SourceHealthDiagnostic] ?? "来源暂不可用";
}
