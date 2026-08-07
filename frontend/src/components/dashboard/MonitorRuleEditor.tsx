"use client";

import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { createMonitorRule, MAX_MONITOR_RULES, MONITOR_RULE_TYPES, type MonitorDraftRule, type MonitorRuleType } from "@/lib/monitorDraft";

type Props = {
  rules: MonitorDraftRule[];
  onChange: (rules: MonitorDraftRule[]) => void;
};

export function MonitorRuleEditor({ rules, onChange }: Props) {
  const update = (key: string, change: Partial<MonitorDraftRule>) =>
    onChange(rules.map((rule) => (rule.key === key ? { ...rule, ...change } : rule)));

  return (
    <div className="sm:col-span-2">
      <div className="flex items-end justify-between gap-3">
        <div><Label>包含与排除规则</Label><p className="mt-1 text-xs text-muted-foreground">人工规则直接进入草稿，AI 候选需管理员审批。</p></div>
        <span className="mono shrink-0 text-xs text-muted-foreground">{rules.length}/{MAX_MONITOR_RULES}</span>
      </div>
      <div className="mt-3 space-y-2">
        {rules.map((rule, index) => (
          <div key={rule.key} className="grid gap-2 rounded-md border border-border p-3 sm:grid-cols-[150px_1fr_auto]">
            <Select value={rule.ruleType} onValueChange={(value) => update(rule.key, { ruleType: value as MonitorRuleType })}>
              <SelectTrigger aria-label={`规则 ${index + 1} 类型`}><SelectValue /></SelectTrigger>
              <SelectContent>{MONITOR_RULE_TYPES.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}</SelectContent>
            </Select>
            <Input aria-label={`规则 ${index + 1} 内容`} value={rule.value} onChange={(event) => update(rule.key, { value: event.target.value })} placeholder={rule.ruleType === "exclude_keyword" ? "例如：招聘" : "例如：AI Agent"} />
            <Button type="button" variant="ghost" size="icon" aria-label={`删除规则 ${index + 1}`} onClick={() => onChange(rules.filter((candidate) => candidate.key !== rule.key))}><Trash2 /></Button>
          </div>
        ))}
      </div>
      <Button type="button" variant="outline" size="sm" className="mt-3 gap-1.5" disabled={rules.length >= MAX_MONITOR_RULES} onClick={() => onChange([...rules, createMonitorRule()])}><Plus />添加规则</Button>
    </div>
  );
}
