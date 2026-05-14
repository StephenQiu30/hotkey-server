"use client";

import { FormEvent, useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { api, Keyword } from "@/lib/api";

export function KeywordsClient() {
  const [items, setItems] = useState<Keyword[]>([]);
  const [keyword, setKeyword] = useState("");
  const [queryTemplate, setQueryTemplate] = useState("");
  const [priority, setPriority] = useState(0);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [busyId, setBusyId] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    setItems(await api<Keyword[]>("/api/keywords"));
  }

  useEffect(() => {
    load().catch((err) => setError(err.message)).finally(() => setLoading(false));
  }, []);

  async function createKeyword(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await api<Keyword>("/api/keywords", {
        method: "POST",
        body: JSON.stringify({ keyword, query_template: queryTemplate || null, enabled: true, priority }),
      });
      setKeyword("");
      setQueryTemplate("");
      setPriority(0);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "新增关键词失败");
    } finally {
      setSaving(false);
    }
  }

  async function toggleKeyword(id: number) {
    setBusyId(id);
    setError(null);
    try {
      await api<Keyword>(`/api/keywords/${id}/toggle`, { method: "POST" });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "切换关键词失败");
    } finally {
      setBusyId(null);
    }
  }

  async function deleteKeyword(id: number) {
    setBusyId(id);
    setError(null);
    try {
      await api<void>(`/api/keywords/${id}`, { method: "DELETE" });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "删除关键词失败");
    } finally {
      setBusyId(null);
    }
  }

  if (loading) return <Skeleton className="h-80 ios-shell-card" />;

  return (
    <div className="grid gap-4">
      <Card className="ios-shell-card">
        <CardHeader>
          <CardTitle>关键词配置</CardTitle>
          <CardDescription>新增关键词后会触发检测输入，系统将据此检索相关热点。</CardDescription>
        </CardHeader>
        <CardContent className="pt-5">
          <form className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_140px_max-content]" onSubmit={createKeyword}>
            <div className="grid gap-2">
              <Label htmlFor="keyword">关键词</Label>
              <Input id="keyword" onChange={(event) => setKeyword(event.target.value)} placeholder="例如 OpenAI" required value={keyword} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="query-template">查询模板</Label>
              <Input id="query-template" onChange={(event) => setQueryTemplate(event.target.value)} placeholder="可选" value={queryTemplate} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="priority">优先级</Label>
              <Input id="priority" onChange={(event) => setPriority(Number(event.target.value))} type="number" value={priority} />
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
        {items.length === 0 ? <p className="p-6 text-sm text-muted-foreground">暂无关键词。</p> : null}
        {items.length > 0 ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>关键词</TableHead>
                <TableHead>模板</TableHead>
                <TableHead>优先级</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.id}>
                  <TableCell className="font-semibold">{item.keyword}</TableCell>
                  <TableCell className="max-w-xs truncate text-muted-foreground">{item.query_template || "-"}</TableCell>
                  <TableCell>{item.priority}</TableCell>
                  <TableCell><Badge variant={item.enabled ? "success" : "muted"}>{item.enabled ? "启用" : "停用"}</Badge></TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-2">
                      <Button disabled={busyId === item.id} onClick={() => toggleKeyword(item.id)} size="sm" type="button" variant="secondary">
                        {busyId === item.id ? "处理中" : "切换"}
                      </Button>
                      <Button disabled={busyId === item.id} onClick={() => deleteKeyword(item.id)} size="sm" type="button" variant="destructive">
                        <Trash2 className="h-4 w-4" />
                        删除
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
