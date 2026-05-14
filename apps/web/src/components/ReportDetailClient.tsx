"use client";

import { useEffect, useState } from "react";
import { Send } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { api, formatDate, Report, statusTone } from "@/lib/api";

export function ReportDetailClient({ id }: { id: string }) {
  const [report, setReport] = useState<Report | null>(null);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    setReport(await api<Report>(`/api/reports/${id}`));
  }

  useEffect(() => {
    load().catch((err) => setError(err.message));
  }, [id]);

  async function sendReport() {
    setSending(true);
    setError(null);
    try {
      setReport(await api<Report>(`/api/reports/${id}/send`, { method: "POST" }));
    } catch (err) {
      setError(err instanceof Error ? err.message : "发送报告失败");
    } finally {
      setSending(false);
    }
  }

  if (!report && !error) return <Skeleton className="h-96 ios-shell-card" />;

  const statusText: Record<string, string> = {
    generated: "已生成",
    sent: "已发送",
    failed: "发送失败",
    filtered: "已过滤",
  };

  return (
    <div className="grid gap-5">
      {error ? (
        <p className="ios-card-muted border-destructive/35 bg-destructive/10 border p-3 text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}
      {report ? (
        <>
          <Card className="ios-shell-card">
            <CardHeader>
              <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                <div>
                  <CardTitle className="text-2xl leading-tight">{report.subject}</CardTitle>
                  <CardDescription className="mt-2">{report.summary || "暂无摘要"}</CardDescription>
                </div>
                <Badge variant={statusTone(report.status)}>{statusText[report.status] || report.status}</Badge>
              </div>
            </CardHeader>
            <CardContent className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
              <div className="flex flex-wrap gap-3 text-sm text-muted-foreground">
                <span>{report.report_type}</span>
                <span>{report.hotspot_count} 条热点</span>
                <span>{formatDate(report.period_start)} - {formatDate(report.period_end)}</span>
                <span>发送：{formatDate(report.sent_at)}</span>
              </div>
              <Button disabled={sending} onClick={sendReport} type="button">
                {sending ? "发送中" : "发送报告"}
                <Send className="h-4 w-4" />
              </Button>
            </CardContent>
          </Card>
          <Card className="ios-shell-card">
            <CardHeader>
              <CardTitle>Markdown 预览</CardTitle>
            </CardHeader>
            <CardContent>
              <pre className="max-h-[640px] overflow-auto whitespace-pre-wrap rounded-lg bg-slate-950 p-4 text-sm leading-7 text-slate-100">{report.content}</pre>
            </CardContent>
          </Card>
        </>
      ) : null}
    </div>
  );
}
