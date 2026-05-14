"use client";

import { FormEvent, useEffect, useState } from "react";
import { Plus } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api, Source } from "@/lib/api";

const sourceTypes = ["rss", "hacker_news", "x_twitter", "bing", "bilibili", "sogou"];

export function SourcesClient() {
  const [items, setItems] = useState<Source[]>([]);
  const [name, setName] = useState("");
  const [sourceType, setSourceType] = useState("rss");
  const [config, setConfig] = useState('{"url":"https://hnrss.org/frontpage","limit":20}');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    setItems(await api<Source[]>("/api/sources"));
  }

  useEffect(() => {
    load().catch((err) => setError(err.message)).finally(() => setLoading(false));
  }, []);

  async function createSource(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await api<Source>("/api/sources", {
        method: "POST",
        body: JSON.stringify({ name, source_type: sourceType, enabled: true, config: JSON.parse(config || "{}") }),
      });
      setName("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "新增来源失败，请检查 JSON 配置。");
    } finally {
      setSaving(false);
    }
  }

  async function toggleSource(id: number) {
    setBusyId(id);
    setError(null);
    try {
      await api<Source>(`/api/sources/${id}/toggle`, { method: "POST" });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "切换来源失败");
    } finally {
      setBusyId(null);
    }
  }

  if (loading) return <Skeleton className="h-80 ios-shell-card" />;

  return (
    <div className="grid gap-4">
      <Card className="ios-shell-card">
        <CardHeader>
          <CardTitle>来源配置</CardTitle>
          <CardDescription>统一适配 RSS、HN、X/Twitter、Bing、Bilibili 与 Sogou-style。</CardDescription>
        </CardHeader>
        <CardContent className="pt-5">
          <form className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_180px_minmax(0,2fr)_max-content]" onSubmit={createSource}>
            <div className="grid gap-2">
              <Label htmlFor="source-name">来源名称</Label>
              <Input id="source-name" onChange={(event) => setName(event.target.value)} placeholder="例如 Hacker News Search" required value={name} />
            </div>
            <div className="grid gap-2">
              <Label>来源类型</Label>
              <Select onValueChange={setSourceType} value={sourceType}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {sourceTypes.map((type) => <SelectItem key={type} value={type}>{type}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="source-config">JSON 配置</Label>
              <Input id="source-config" onChange={(event) => setConfig(event.target.value)} value={config} />
            </div>
            <div className="flex items-end">
              <Button className="w-full xl:w-auto" disabled={saving} type="submit">
                {saving ? "新增中" : "新增"}
                <Plus className="h-4 w-4" />
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
        {items.length === 0 ? <p className="p-6 text-sm text-muted-foreground">暂无来源。</p> : null}
        {items.length > 0 ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>类型</TableHead>
                <TableHead>配置</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.id}>
                  <TableCell className="font-semibold">{item.name}</TableCell>
                  <TableCell>{item.source_type}</TableCell>
                  <TableCell className="max-w-xs truncate font-mono text-xs text-muted-foreground">{JSON.stringify(item.config)}</TableCell>
                  <TableCell><Badge variant={item.enabled ? "success" : "muted"}>{item.enabled ? "启用" : "停用"}</Badge></TableCell>
                  <TableCell className="text-right">
                    <Button disabled={busyId === item.id} onClick={() => toggleSource(item.id)} size="sm" type="button" variant="secondary">
                      {busyId === item.id ? "处理中" : "切换"}
                    </Button>
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
