"use client";

import { FormEvent, useEffect, useState } from "react";
import { Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { api, Setting } from "@/lib/api";

export function SettingsClient() {
  const [items, setItems] = useState<Setting[]>([]);
  const [key, setKey] = useState("daily_report");
  const [value, setValue] = useState('{"enabled":true}');
  const [description, setDescription] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    setItems(await api<Setting[]>("/api/settings"));
  }

  useEffect(() => {
    load().catch((err) => setError(err.message)).finally(() => setLoading(false));
  }, []);

  async function saveSetting(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await api<Setting>(`/api/settings/${key}`, {
        method: "PUT",
        body: JSON.stringify({ value: JSON.parse(value || "{}"), description: description || null }),
      });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "保存设置失败，请检查 JSON。");
    } finally {
      setSaving(false);
    }
  }

  if (loading) return <Skeleton className="h-80 ios-shell-card" />;

  return (
    <div className="grid gap-4">
      <Card className="ios-shell-card">
        <CardHeader>
          <CardTitle>系统设置</CardTitle>
          <CardDescription>支持更新本地运行参数，敏感凭据仍需通过环境变量注入。</CardDescription>
        </CardHeader>
        <CardContent className="pt-5">
          <form className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_minmax(0,1fr)_max-content]" onSubmit={saveSetting}>
            <div className="grid gap-2">
              <Label htmlFor="setting-key">Key</Label>
              <Input id="setting-key" onChange={(event) => setKey(event.target.value)} required value={key} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="setting-value">JSON value</Label>
              <Textarea className="min-h-11 lg:min-h-11" id="setting-value" onChange={(event) => setValue(event.target.value)} value={value} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="setting-desc">描述</Label>
              <Input id="setting-desc" onChange={(event) => setDescription(event.target.value)} value={description} />
            </div>
            <div className="flex items-end">
              <Button className="w-full xl:w-auto" disabled={saving} type="submit">
                {saving ? "保存中" : "保存"}
                <Save className="h-4 w-4" />
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
        {items.length === 0 ? <p className="p-6 text-sm text-muted-foreground">暂无设置。</p> : null}
        {items.length > 0 ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Key</TableHead>
                <TableHead>Value</TableHead>
                <TableHead>描述</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.key}>
                  <TableCell className="font-semibold">{item.key}</TableCell>
                  <TableCell className="max-w-md truncate font-mono text-xs text-muted-foreground">{JSON.stringify(item.value)}</TableCell>
                  <TableCell>{item.description || "-"}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : null}
      </Card>
    </div>
  );
}
