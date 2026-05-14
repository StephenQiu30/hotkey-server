"use client";

import Link from "next/link";
import { FormEvent, useEffect, useState } from "react";
import { FileText, Send } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api, formatDate, Page, Report, statusTone } from "@/lib/api";

export function ReportsClient() {
  const [items, setItems] = useState<Report[]>([]);
  const [reportType, setReportType] = useState<"daily" | "weekly">("daily");
  const [periodStart, setPeriodStart] = useState("");
  const [send, setSend] = useState(false);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    const page = await api<Page<Report>>("/api/reports");
    setItems(page.items);
  }

  useEffect(() => {
    load().catch((err) => setError(err.message)).finally(() => setLoading(false));
  }, []);

  async function createReport(event: FormEvent) {
    event.preventDefault();
    setCreating(true);
    setError(null);
    try {
      await api<Report>("/api/reports", {
        method: "POST",
        body: JSON.stringify({ report_type: reportType, period_start: periodStart || null, send }),
      });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "生成报告失败");
    } finally {
      setCreating(false);
    }
  }

  async function sendReport(id: number) {
    setBusyId(id);
    setError(null);
    try {
      await api<Report>(`/api/reports/${id}/send`, { method: "POST" });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "发送报告失败");
    } finally {
      setBusyId(null);
    }
  }

  if (loading) return <Skeleton className="h-80 ios-shell-card" />;

  const statusText: Record<string, string> = {
    generated: "已生成",
    sent: "已发送",
    failed: "发送失败",
    filtered: "已过滤",
  };

  return (
    <div className="grid gap-4">
      <Card className="ios-shell-card">
        <CardHeader>
          <CardTitle>报告编排</CardTitle>
          <CardDescription>按周期生成日报或周报，并可快速触发发送。</CardDescription>
        </CardHeader>
        <CardContent className="pt-5">
          <form className="grid gap-3 md:grid-cols-2 xl:grid-cols-[180px_200px_160px_max-content]" onSubmit={createReport}>
            <div className="grid gap-2">
              <Label>类型</Label>
              <Select onValueChange={(value) => setReportType(value as "daily" | "weekly")} value={reportType}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="daily">日报</SelectItem>
                  <SelectItem value="weekly">周报</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="period-start">周期开始</Label>
              <Input id="period-start" onChange={(event) => setPeriodStart(event.target.value)} type="date" value={periodStart} />
            </div>
            <div className="grid gap-2">
              <Label>生成后发送</Label>
              <Select onValueChange={(value) => setSend(value === "true")} value={String(send)}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="false">只生成</SelectItem>
                  <SelectItem value="true">生成并发送</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-end">
              <Button className="w-full xl:w-auto" disabled={creating} type="submit">
                {creating ? "生成中" : "生成报告"}
                <FileText className="h-4 w-4" />
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {error ? (
        <p className="ios-card-muted border-destructive/35 bg-destructive/10 border p-3 text-sm text-destructive" role="alert">
          {error}
        </p>
      ) : null}

      <Card className="ios-shell-card">
        {items.length === 0 ? <p className="p-6 text-sm text-muted-foreground">暂无报告。选择类型后生成第一份日报或周报。</p> : null}
        {items.length > 0 ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>报告</TableHead>
                <TableHead>类型</TableHead>
                <TableHead>热点数</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>周期</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.id}>
                  <TableCell className="max-w-sm truncate font-semibold">
                    <Link className="ios-focus-ring rounded-md hover:text-primary" href={`/app/reports/${item.id}`}>{item.subject}</Link>
                  </TableCell>
                  <TableCell>{item.report_type}</TableCell>
                  <TableCell>{item.hotspot_count}</TableCell>
                  <TableCell><Badge variant={statusTone(item.status)}>{statusText[item.status] || item.status}</Badge></TableCell>
                  <TableCell>{formatDate(item.period_start)} - {formatDate(item.period_end)}</TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-2">
                      <Button asChild size="sm" variant="secondary">
                        <Link href={`/app/reports/${item.id}`}>预览</Link>
                      </Button>
                      <Button disabled={busyId === item.id} onClick={() => sendReport(item.id)} size="sm" type="button">
                        {busyId === item.id ? "发送中" : "发送"}
                        <Send className="h-4 w-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : null}
      </Card>
    </div>
  );
}
