"use client";

import { type FormEvent, useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { MONITOR_RULE_TYPES, type MonitorRuleType } from "@/lib/monitorDraft";

type Props = {
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (candidate: { ruleType: MonitorRuleType; value: string }) => void;
  open: boolean;
};

export function AICandidateDialog({ busy, onOpenChange, onSubmit, open }: Props) {
  const [ruleType, setRuleType] = useState<MonitorRuleType>("keyword");
  const [value, setValue] = useState("");
  useEffect(() => { if (!open) { setRuleType("keyword"); setValue(""); } }, [open]);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (value.trim()) onSubmit({ ruleType, value: value.trim() });
  };
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="sm:max-w-md"><DialogHeader><DialogTitle>导入 AI 扩展候选</DialogTitle><DialogDescription>候选只进入当前草稿，不会自动发布。当 AI 不可用时，人工规则仍可独立运行。</DialogDescription></DialogHeader><form onSubmit={submit} className="space-y-4"><div><Label htmlFor="candidate-type">候选类型</Label><Select value={ruleType} onValueChange={(next) => setRuleType(next as MonitorRuleType)}><SelectTrigger id="candidate-type" className="mt-2"><SelectValue /></SelectTrigger><SelectContent>{MONITOR_RULE_TYPES.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}</SelectContent></Select></div><div><Label htmlFor="candidate-value">候选内容</Label><Input id="candidate-value" className="mt-2" value={value} onChange={(event) => setValue(event.target.value)} maxLength={160} placeholder="例如：agentic workflow" /></div><div className="rounded-md border border-border bg-muted/30 p-3 text-xs text-muted-foreground">来源：AI 扩展 · 风险：中 · 初始状态：待审批</div><DialogFooter><Button type="button" variant="outline" onClick={() => onOpenChange(false)}>取消</Button><Button type="submit" disabled={busy || !value.trim()}>{busy && <Loader2 className="animate-spin" />}加入待审批</Button></DialogFooter></form></DialogContent></Dialog>;
}
