"use client";

import { useEffect, useState } from "react";
import { Play } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api, CheckRun, formatDate, Page, statusTone } from "@/lib/api";

export function RunsClient() {
  const [items, setItems] = useState<CheckRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    const page = await api<Page<CheckRun>>("/api/check-runs");
    setItems(page.items);
  }

  useEffect(() => {
    load().catch((err) => setError(err.message)).finally(() => setLoading(false));
  }, []);

  async function triggerRun() {
    setRunning(true);
    setError(null);
    try {
      await api<CheckRun>("/api/check-runs", { method: "POST", body: JSON.stringify({ trigger_type: "manual" }) });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "触发失败");
    } finally {
      setRunning(false);
    }
  }

  if (loading) return <Skeleton className="h-80 ios-shell-card" />;

  const statusLabel: Record<string, string> = {
    active: "活动",
    filtered: "已过滤",
  };

  return (
    <div className="grid gap-4">
      <Card className="ios-shell-card">
        <CardHeader>
          <CardTitle>任务编排</CardTitle>
          <CardDescription>手动触发一次检测，结果回填到热点列表、通知和报告。</CardDescription>
        </CardHeader>
        <CardContent>
          <Button disabled={running} onClick={triggerRun} type="button">
            {running ? "运行中" : "手动检查"}
            <Play className="h-4 w-4" />
          </Button>
        </CardContent>
      </Card>
      {error ? (
        <p className="ios-card-muted border-destructive/35 bg-destructive/10 border p-3 text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}
      <Card className="ios-shell-card">
        {items.length === 0 ? <p className="p-6 text-sm text-muted-foreground">暂无任务记录。</p> : null}
        {items.length > 0 ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>触发</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>成功</TableHead>
                <TableHead>失败</TableHead>
                <TableHead>开始</TableHead>
                <TableHead>结束</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.id}>
                  <TableCell>{item.trigger_type}</TableCell>
                  <TableCell><Badge variant={statusTone(item.status)}>{statusLabel[item.status] || item.status}</Badge></TableCell>
                  <TableCell>{item.success_count}</TableCell>
                  <TableCell>{item.failure_count}</TableCell>
                  <TableCell>{formatDate(item.started_at)}</TableCell>
                  <TableCell>{formatDate(item.finished_at)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : null}
      </Card>
    </div>
  );
}
