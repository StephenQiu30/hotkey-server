"use client";

import { memo } from "react";
import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type {
  MonitorIntentClauseField,
  MonitorIntentClauseOperator,
  MonitorIntentDraftForm,
  MonitorIntentExampleLabel,
} from "@/lib/monitorIntentDraft";

const clauseOperators: Array<{ label: string; value: MonitorIntentClauseOperator }> = [
  { label: "必须满足", value: "must" },
  { label: "优先召回", value: "should" },
  { label: "必须排除", value: "must_not" },
];

const clauseFields: Array<{ label: string; value: MonitorIntentClauseField }> = [
  { label: "词语", value: "term" },
  { label: "短语", value: "phrase" },
  { label: "动作", value: "action" },
  { label: "地点", value: "location" },
  { label: "语言", value: "language" },
  { label: "地区", value: "region" },
  { label: "来源", value: "source" },
  { label: "时间窗口", value: "time_window" },
];

const exampleLabels: Array<{ label: string; value: MonitorIntentExampleLabel }> = [
  { label: "正例", value: "positive" },
  { label: "反例", value: "negative" },
];

type MonitorIntentEditorProps = {
  disabled?: boolean;
  form: MonitorIntentDraftForm;
  onChange: (next: MonitorIntentDraftForm) => void;
};

export const MonitorIntentEditor = memo(function MonitorIntentEditor({
  disabled = false,
  form,
  onChange,
}: MonitorIntentEditorProps) {
  return (
    <div className="space-y-5">
      <Card className="gap-4 p-5">
        <div>
          <h2 className="text-base font-semibold">自然语言目标</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            描述希望持续发现的事件。这里只定义相关性，不判断报道是否真实。
          </p>
        </div>
        <div>
          <Label htmlFor="monitor-intent-objective">监控目标</Label>
          <textarea
            id="monitor-intent-objective"
            className="mt-2 min-h-28 w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
            disabled={disabled}
            maxLength={2000}
            onChange={(event) => onChange({ ...form, objective: event.target.value })}
            placeholder="例如：跟踪全球主要 AI 厂商正式发布的新模型、Agent 产品及开放 API"
            value={form.objective}
          />
          <p className="mt-1 text-right text-xs text-muted-foreground">
            {[...form.objective].length} / 2000
          </p>
        </div>
      </Card>

      <Card className="gap-4 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">结构化条件</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              操作符和字段分别保存，系统不会从文字猜测“必须”或“排除”。
            </p>
          </div>
          <Button
            disabled={disabled || form.clauses.length >= 128}
            onClick={() =>
              onChange({
                ...form,
                clauses: [...form.clauses, { operator: "must", field: "term", value: "" }],
              })
            }
            size="sm"
            type="button"
            variant="outline"
          >
            <Plus />
            添加条件
          </Button>
        </div>
        {form.clauses.length === 0 ? (
          <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
            暂无结构化条件；可只保存自然语言目标，也可添加硬条件提高可解释性。
          </p>
        ) : (
          <div className="space-y-3">
            {form.clauses.map((clause, index) => (
              <div
                className="grid gap-2 rounded-md border border-border p-3 sm:grid-cols-[150px_150px_minmax(0,1fr)_40px] sm:items-center"
                key={`clause-${index}`}
              >
                <Select
                  disabled={disabled}
                  onValueChange={(value) => {
                    const clauses = [...form.clauses];
                    clauses[index] = { ...clause, operator: value as MonitorIntentClauseOperator };
                    onChange({ ...form, clauses });
                  }}
                  value={clause.operator}
                >
                  <SelectTrigger aria-label={`条件 ${index + 1} 操作符`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {clauseOperators.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select
                  disabled={disabled}
                  onValueChange={(value) => {
                    const clauses = [...form.clauses];
                    clauses[index] = { ...clause, field: value as MonitorIntentClauseField };
                    onChange({ ...form, clauses });
                  }}
                  value={clause.field}
                >
                  <SelectTrigger aria-label={`条件 ${index + 1} 字段`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {clauseFields.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Input
                  aria-label={`条件 ${index + 1} 内容`}
                  disabled={disabled}
                  maxLength={512}
                  onChange={(event) => {
                    const clauses = [...form.clauses];
                    clauses[index] = { ...clause, value: event.target.value };
                    onChange({ ...form, clauses });
                  }}
                  placeholder="条件内容"
                  value={clause.value}
                />
                <Button
                  aria-label={`删除条件 ${index + 1}`}
                  disabled={disabled}
                  onClick={() =>
                    onChange({ ...form, clauses: form.clauses.filter((_, item) => item !== index) })
                  }
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <Trash2 />
                </Button>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card className="gap-4 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">实体消歧</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              显式记录实体身份、别名和歧义说明，避免同名词被错误合并。
            </p>
          </div>
          <Button
            disabled={disabled || form.entities.length >= 64}
            onClick={() =>
              onChange({
                ...form,
                entities: [
                  ...form.entities,
                  { canonicalId: "", displayName: "", aliasesText: "", ambiguityNote: "" },
                ],
              })
            }
            size="sm"
            type="button"
            variant="outline"
          >
            <Plus />
            添加实体
          </Button>
        </div>
        {form.entities.map((entity, index) => (
          <div className="grid gap-3 rounded-md border border-border p-4 md:grid-cols-2" key={`entity-${index}`}>
            <div>
              <Label htmlFor={`intent-entity-id-${index}`}>规范身份</Label>
              <Input
                className="mt-2"
                disabled={disabled}
                id={`intent-entity-id-${index}`}
                onChange={(event) => {
                  const entities = [...form.entities];
                  entities[index] = { ...entity, canonicalId: event.target.value };
                  onChange({ ...form, entities });
                }}
                placeholder="例如 openai"
                value={entity.canonicalId}
              />
            </div>
            <div>
              <Label htmlFor={`intent-entity-name-${index}`}>显示名称</Label>
              <Input
                className="mt-2"
                disabled={disabled}
                id={`intent-entity-name-${index}`}
                onChange={(event) => {
                  const entities = [...form.entities];
                  entities[index] = { ...entity, displayName: event.target.value };
                  onChange({ ...form, entities });
                }}
                placeholder="例如 OpenAI"
                value={entity.displayName}
              />
            </div>
            <div>
              <Label htmlFor={`intent-entity-aliases-${index}`}>别名（逗号分隔）</Label>
              <Input
                className="mt-2"
                disabled={disabled}
                id={`intent-entity-aliases-${index}`}
                onChange={(event) => {
                  const entities = [...form.entities];
                  entities[index] = { ...entity, aliasesText: event.target.value };
                  onChange({ ...form, entities });
                }}
                placeholder="ChatGPT, Open AI"
                value={entity.aliasesText}
              />
            </div>
            <div>
              <Label htmlFor={`intent-entity-note-${index}`}>歧义说明</Label>
              <div className="mt-2 flex gap-2">
                <Input
                  disabled={disabled}
                  id={`intent-entity-note-${index}`}
                  onChange={(event) => {
                    const entities = [...form.entities];
                    entities[index] = { ...entity, ambiguityNote: event.target.value };
                    onChange({ ...form, entities });
                  }}
                  placeholder="例如：指公司，不指产品名称"
                  value={entity.ambiguityNote}
                />
                <Button
                  aria-label={`删除实体 ${index + 1}`}
                  disabled={disabled}
                  onClick={() =>
                    onChange({ ...form, entities: form.entities.filter((_, item) => item !== index) })
                  }
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <Trash2 />
                </Button>
              </div>
            </div>
          </div>
        ))}
      </Card>

      <Card className="gap-4 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">正例与反例</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              样本只用于解释相关性边界，不作为真实性标签。
            </p>
          </div>
          <Button
            disabled={disabled || form.examples.length >= 64}
            onClick={() =>
              onChange({
                ...form,
                examples: [...form.examples, { label: "positive", text: "" }],
              })
            }
            size="sm"
            type="button"
            variant="outline"
          >
            <Plus />
            添加样本
          </Button>
        </div>
        {form.examples.map((example, index) => (
          <div
            className="grid gap-2 rounded-md border border-border p-3 sm:grid-cols-[120px_minmax(0,1fr)_40px] sm:items-center"
            key={`example-${index}`}
          >
            <Select
              disabled={disabled}
              onValueChange={(value) => {
                const examples = [...form.examples];
                examples[index] = { ...example, label: value as MonitorIntentExampleLabel };
                onChange({ ...form, examples });
              }}
              value={example.label}
            >
              <SelectTrigger aria-label={`样本 ${index + 1} 类型`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {exampleLabels.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Input
              aria-label={`样本 ${index + 1} 内容`}
              disabled={disabled}
              onChange={(event) => {
                const examples = [...form.examples];
                examples[index] = { ...example, text: event.target.value };
                onChange({ ...form, examples });
              }}
              placeholder={example.label === "positive" ? "应该被召回的例子" : "不应该被召回的例子"}
              value={example.text}
            />
            <Button
              aria-label={`删除样本 ${index + 1}`}
              disabled={disabled}
              onClick={() =>
                onChange({ ...form, examples: form.examples.filter((_, item) => item !== index) })
              }
              size="icon"
              type="button"
              variant="ghost"
            >
              <Trash2 />
            </Button>
          </div>
        ))}
      </Card>
    </div>
  );
});
