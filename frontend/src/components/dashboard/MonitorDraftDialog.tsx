"use client";

import { type FormEvent } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { MonitorRegion } from "@/lib/domainEnums";
import {
  MAX_MONITOR_SOURCES,
  MONITOR_LIMITS,
  selectAllMonitorSources,
  toggleMonitorSource,
  type MonitorDraftForm,
} from "@/lib/monitorDraft";

type MonitorDraftDialogProps = {
  busy: boolean;
  form: MonitorDraftForm;
  mode: "create" | "edit";
  onFormChange: (form: MonitorDraftForm) => void;
  onOpenChange: (open: boolean) => void;
  onSubmit: (event: FormEvent) => void;
  open: boolean;
  sources: HotKeyAPI.SourceReadResponse[];
};

const numberFields = [
  ["采集间隔（秒）", "interval", MONITOR_LIMITS.interval.min, MONITOR_LIMITS.interval.max, MONITOR_LIMITS.interval.step],
  ["相关性阈值", "relevance", MONITOR_LIMITS.relevance.min, MONITOR_LIMITS.relevance.max, 1],
  ["事件阈值", "event", MONITOR_LIMITS.event.min, MONITOR_LIMITS.event.max, 1],
  ["保留天数", "retention", MONITOR_LIMITS.retention.min, MONITOR_LIMITS.retention.max, 1],
] as const;

export function MonitorDraftDialog({ busy, form, mode, onFormChange, onOpenChange, onSubmit, open, sources }: MonitorDraftDialogProps) {
  const availableIds = sources.flatMap((source) => source.id == null ? [] : [source.id]);
  const selectAllIds = selectAllMonitorSources(availableIds);
  const allSelected = selectAllIds.length > 0 && selectAllIds.every((id) => form.sourceIds.includes(id)) && form.sourceIds.length === selectAllIds.length;
  const selectionState = allSelected ? true : form.sourceIds.length > 0 ? "indeterminate" : false;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-hidden p-0 sm:max-w-xl">
        <DialogHeader className="border-b border-border px-6 py-5">
          <DialogTitle>{mode === "create" ? "新建监控草稿" : "编辑监控草稿"}</DialogTitle>
          <DialogDescription>配置一个关键词规则、采集节奏、阈值和已启用数据来源。</DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit}>
          <div className="grid max-h-[calc(90vh-9rem)] gap-4 overflow-y-auto px-6 py-5 sm:grid-cols-2">
            <div className="sm:col-span-2">
              <Label htmlFor="monitor-name">监控名称</Label>
              <Input id="monitor-name" className="mt-2" value={form.name} onChange={(event) => onFormChange({ ...form, name: event.target.value })} placeholder="例如：AI 产品发布" />
            </div>
            <div className="sm:col-span-2">
              <Label htmlFor="monitor-query">关键词规则</Label>
              <Input id="monitor-query" className="mt-2" value={form.query} onChange={(event) => onFormChange({ ...form, query: event.target.value })} placeholder="AI Agent OR 智能体 OR agentic" />
            </div>
            <div className="sm:col-span-2">
              <Label htmlFor="monitor-description">说明</Label>
              <Input id="monitor-description" className="mt-2" value={form.description} onChange={(event) => onFormChange({ ...form, description: event.target.value })} placeholder="说明这个监控关注什么" />
            </div>
            <div>
              <Label htmlFor="monitor-language">语言</Label>
              <Select value={form.language} onValueChange={(value) => onFormChange({ ...form, language: value })}>
                <SelectTrigger id="monitor-language" className="mt-2"><SelectValue /></SelectTrigger>
                <SelectContent><SelectItem value="zh">中文</SelectItem><SelectItem value="en">English</SelectItem></SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="monitor-region">地区</Label>
              <Select value={form.region} onValueChange={(value) => onFormChange({ ...form, region: value as MonitorRegion })}>
                <SelectTrigger id="monitor-region" className="mt-2"><SelectValue /></SelectTrigger>
                <SelectContent><SelectItem value={MonitorRegion.China}>中国</SelectItem><SelectItem value={MonitorRegion.UnitedStates}>美国</SelectItem><SelectItem value={MonitorRegion.Global}>全球</SelectItem></SelectContent>
              </Select>
            </div>
            {numberFields.map(([label, key, min, max, step]) => (
              <div key={key}>
                <Label htmlFor={`monitor-${key}`}>{label}</Label>
                <Input id={`monitor-${key}`} type="number" min={min} max={max} step={step} className="mono mt-2" value={form[key]} onChange={(event) => onFormChange({ ...form, [key]: Number(event.target.value) })} />
              </div>
            ))}
            <div className="sm:col-span-2">
              <div className="flex items-end justify-between gap-3">
                <div><Label>数据来源</Label><p className="mt-1 text-xs text-muted-foreground">选择已启用来源，最多 {MAX_MONITOR_SOURCES} 个。</p></div>
                <span className="mono shrink-0 text-xs text-muted-foreground">已选 {form.sourceIds.length}/{MAX_MONITOR_SOURCES}</span>
              </div>
              <div className="mt-2 overflow-hidden rounded-md border border-border">
                {sources.length > 0 && (
                  <div className="flex items-center gap-3 border-b border-border bg-muted/30 px-3 py-2.5 text-sm">
                    <label className="flex cursor-pointer items-center gap-3 font-medium">
                      <Checkbox aria-label="全选数据来源" checked={selectionState} onCheckedChange={(checked) => onFormChange({ ...form, sourceIds: checked === true ? selectAllIds : [] })} />
                      <span>全选</span>
                    </label>
                    <Button type="button" variant="ghost" size="sm" className="ml-auto h-7 px-2 text-xs" disabled={!form.sourceIds.length} onClick={() => onFormChange({ ...form, sourceIds: [] })}>清空</Button>
                  </div>
                )}
                <div className="max-h-64 divide-y divide-border overflow-y-auto">
                  {sources.length ? sources.map((source) => (
                    <label key={source.id} className="flex cursor-pointer items-center gap-3 px-3 py-3 text-sm has-[[data-disabled]]:cursor-not-allowed has-[[data-disabled]]:opacity-50">
                      <Checkbox aria-label={`选择 ${source.name ?? `来源 #${source.id}`}`} checked={source.id != null && form.sourceIds.includes(source.id)} onCheckedChange={(checked) => source.id != null && onFormChange({ ...form, sourceIds: toggleMonitorSource(form.sourceIds, source.id, checked === true) })} disabled={source.id != null && !form.sourceIds.includes(source.id) && form.sourceIds.length >= MAX_MONITOR_SOURCES} />
                      <span>{source.name}</span><span className="mono ml-auto text-xs text-muted-foreground">{source.source_type}</span>
                    </label>
                  )) : <p className="p-4 text-xs text-muted-foreground">请先在来源管理中创建并启用来源。</p>}
                </div>
              </div>
            </div>
          </div>
          <DialogFooter className="border-t border-border px-6 py-4">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>取消</Button>
            <Button type="submit" disabled={busy || !form.query.trim() || !form.sourceIds.length}>
              {busy && <Loader2 className="animate-spin" />}{busy ? "保存中" : mode === "create" ? "创建草稿" : "保存草稿"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
