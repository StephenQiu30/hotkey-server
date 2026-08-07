"use client";

import { Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { monitorStatusLabel } from "@/lib/domainPresentation";

type MonitorDetailDialogProps = {
  approvingRuleID?: number;
  canAdmin?: boolean;
  error?: string;
  history: HotKeyAPI.MonitorConfigResponse[];
  loading: boolean;
  monitor?: HotKeyAPI.MonitorResponse;
  onCandidateApproval?: (rule: HotKeyAPI.MonitorRuleResponse, approval: "approved" | "rejected") => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
};

const stateLabels: Record<string, string> = { draft: "草稿", published: "已发布", superseded: "已替代" };
const intervalLabel = (seconds?: number) => seconds && seconds % 60 === 0 ? `每 ${seconds / 60} 分钟采集` : seconds ? `每 ${seconds} 秒采集` : "尚未配置采集";

export function MonitorDetailDialog({ approvingRuleID, canAdmin, error, history, loading, monitor, onCandidateApproval, onOpenChange, open }: MonitorDetailDialogProps) {
  const config = monitor?.draft ?? monitor?.published;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-hidden p-0 sm:max-w-2xl">
        <DialogHeader className="border-b border-border px-6 py-5"><DialogTitle>监控详情</DialogTitle><DialogDescription>查看当前运行摘要、规则、来源和不可变版本历史。</DialogDescription></DialogHeader>
        <div
          aria-label="监控详情内容"
          className="max-h-[calc(90vh-8rem)] overflow-y-auto px-6 py-5"
          role="region"
          tabIndex={0}
        >
          {loading ? <div className="flex h-48 items-center justify-center"><Loader2 className="animate-spin text-muted-foreground" /></div> : error ? <div role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">{error}</div> : monitor ? (
            <div className="space-y-5">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div><h3 className="text-base font-medium">{monitor.name || `监控 #${monitor.id}`}</h3><p className="mt-1 text-sm text-muted-foreground">{monitor.description || "暂无说明"}</p></div>
                <Badge variant="outline">{monitorStatusLabel(monitor.status)}</Badge>
              </div>
              <div className="grid gap-3 sm:grid-cols-3">
                <Card className="gap-2 py-4"><CardHeader className="px-4"><CardTitle className="text-xs text-muted-foreground">运行状态</CardTitle></CardHeader><CardContent className="px-4 text-sm font-medium">{monitor.status === "active" ? "将在下一窗口采集" : monitor.status === "paused" ? "已停止新增计划" : monitor.status === "archived" ? "已退出运行" : "等待发布"}</CardContent></Card>
                <Card className="gap-2 py-4"><CardHeader className="px-4"><CardTitle className="text-xs text-muted-foreground">采集计划</CardTitle></CardHeader><CardContent className="px-4 text-sm font-medium">{intervalLabel(monitor.published?.collection_interval_seconds ?? config?.collection_interval_seconds)}</CardContent></Card>
                <Card className="gap-2 py-4"><CardHeader className="px-4"><CardTitle className="text-xs text-muted-foreground">当前修订</CardTitle></CardHeader><CardContent className="mono px-4 text-sm font-medium">{config?.revision ? `修订 ${config.revision}` : "—"}</CardContent></Card>
              </div>
              <section aria-labelledby="monitor-rules-title"><h3 id="monitor-rules-title" className="text-sm font-medium">规则与阈值</h3><div className="mt-3 rounded-md border border-border">
                {(config?.rules ?? []).map((rule) => <div key={rule.id ?? rule.value} className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3 text-sm last:border-b-0"><div className="min-w-0 flex-1"><p className="break-words">{rule.value}</p><p className="mt-1 text-xs text-muted-foreground">{rule.origin === "ai" ? `AI 扩展候选 · 语言 ${(config?.languages ?? []).join("/") || "未指定"} · 风险 ${Number(rule.weight ?? 0) > 60 ? "高" : "中"}` : "人工规则 · 优先生效"}</p></div><Badge variant="secondary">{rule.rule_type ?? "keyword"}</Badge><Badge variant={rule.approval_status === "pending" ? "outline" : "secondary"}>{rule.approval_status === "pending" ? "待审批" : rule.approval_status === "rejected" ? "已拒绝" : "已批准"}</Badge>{canAdmin && rule.origin === "ai" && rule.approval_status === "pending" && <div className="flex gap-2"><Button size="sm" variant="outline" disabled={approvingRuleID === rule.id} onClick={() => onCandidateApproval?.(rule, "rejected")}>拒绝</Button><Button size="sm" disabled={approvingRuleID === rule.id} onClick={() => onCandidateApproval?.(rule, "approved")}>{approvingRuleID === rule.id && <Loader2 className="animate-spin" />}批准</Button></div>}</div>)}
                {!config?.rules?.length && <p className="px-4 py-3 text-sm text-muted-foreground">暂无规则</p>}
              </div><p className="mt-3 text-xs text-muted-foreground">相关性 {config?.relevance_threshold ?? "—"} · 事件 {config?.event_threshold ?? "—"} · 保留 {config?.retention_days ?? "—"} 天</p></section>
              <Separator />
              <section aria-labelledby="monitor-sources-title"><h3 id="monitor-sources-title" className="text-sm font-medium">数据来源</h3><div className="mt-3 flex flex-wrap gap-2">{(config?.sources ?? []).map((source) => <Badge key={source.id ?? source.source_connection_id} variant="outline">{source.name || `来源 #${source.source_connection_id}`}</Badge>)}{!config?.sources?.length && <span className="text-sm text-muted-foreground">暂无来源</span>}</div></section>
              <Separator />
              <section aria-labelledby="monitor-history-title"><h3 id="monitor-history-title" className="text-sm font-medium">版本历史</h3><div className="mt-3 divide-y divide-border overflow-hidden rounded-md border border-border">{history.map((version) => <div key={version.id} className="flex flex-wrap items-center gap-3 px-4 py-3 text-sm"><span className="mono font-medium">修订 {version.revision}</span><Badge variant="secondary">{stateLabels[version.state ?? ""] ?? version.state}</Badge><span className="ml-auto text-xs text-muted-foreground">{version.published_at ? new Date(version.published_at).toLocaleString("zh-CN") : "尚未发布"}</span></div>)}{!history.length && <p className="px-4 py-3 text-sm text-muted-foreground">暂无版本记录</p>}</div></section>
            </div>
          ) : null}
        </div>
        <DialogFooter className="border-t border-border px-6 py-4"><Button variant="outline" onClick={() => onOpenChange(false)}>关闭</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
