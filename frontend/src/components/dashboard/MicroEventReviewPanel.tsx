"use client";

import { FormEvent, useState } from "react";
import { Loader2, Save } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  postMicroEventsIdEvidence,
  postMicroEventsIdFeedback,
} from "@/services/hotkey/hotkey-server/microEvents";

const relations = ["asserts", "attributes_to", "mentions", "contradicts", "corrects", "withdraws", "unknown"] as const;
const governanceActions = ["close", "reopen", "merge", "split", "move_member"] as const;

function mutationHeaders(version: number) {
  return {
    "Content-Type": "application/json",
    "If-Match": `"v${version}"`,
    "Idempotency-Key": crypto.randomUUID(),
  };
}

function positiveInteger(value: FormDataEntryValue | null) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : 0;
}

export function MicroEventReviewPanel({ event, onCompleted }: { event: HotKeyAPI.MicroEventResponseDTO; onCompleted: () => Promise<void> }) {
  const [busy, setBusy] = useState<"evidence" | "governance">();
  const [error, setError] = useState<string>();
  const [relation, setRelation] = useState<(typeof relations)[number]>("asserts");
  const [action, setAction] = useState<(typeof governanceActions)[number]>("close");

  const submitEvidence = async (formEvent: FormEvent<HTMLFormElement>) => {
    formEvent.preventDefault();
    if (!event.id || !event.version) return;
    setBusy("evidence");
    setError(undefined);
    const form = new FormData(formEvent.currentTarget);
    try {
      await postMicroEventsIdEvidence(
        { id: event.id },
        {
          expected_event_version: event.version,
          document_version_id: positiveInteger(form.get("document_version_id")),
          text_quote_selector_id: positiveInteger(form.get("text_quote_selector_id")),
          subject: String(form.get("subject") ?? "").trim(),
          predicate: String(form.get("predicate") ?? "").trim(),
          object: String(form.get("object") ?? "").trim(),
          relation,
          qualifiers: [],
        },
        { headers: mutationHeaders(event.version) },
      );
      formEvent.currentTarget.reset();
      setRelation("asserts");
      await onCompleted();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "证据关系保存失败");
    } finally {
      setBusy(undefined);
    }
  };

  const submitGovernance = async (formEvent: FormEvent<HTMLFormElement>) => {
    formEvent.preventDefault();
    if (!event.id || !event.version) return;
    setBusy("governance");
    setError(undefined);
    const form = new FormData(formEvent.currentTarget);
    try {
      await postMicroEventsIdFeedback(
        { id: event.id },
        {
          expected_event_version: event.version,
          action,
          membership_decision_id: positiveInteger(form.get("membership_decision_id")) || undefined,
          content_family_id: positiveInteger(form.get("content_family_id")) || undefined,
          expected_member_version: positiveInteger(form.get("expected_member_version")) || undefined,
          target_micro_event_id: positiveInteger(form.get("target_micro_event_id")) || undefined,
          expected_target_event_version: positiveInteger(form.get("expected_target_event_version")) || undefined,
          reason_code: String(form.get("reason_code") ?? "").trim(),
          note: String(form.get("note") ?? "").trim() || undefined,
        },
        { headers: mutationHeaders(event.version) },
      );
      await onCompleted();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "事件治理反馈保存失败");
    } finally {
      setBusy(undefined);
    }
  };

  return (
    <Card>
      <CardHeader>
        <h3 className="font-semibold">人工复核</h3>
        <p className="text-sm text-muted-foreground">只追加版本化事实；不会修改原始模型结果，也不会输出真假结论。</p>
      </CardHeader>
      <CardContent className="space-y-7">
        {error ? <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert> : null}
        <form className="space-y-4" onSubmit={submitEvidence}>
          <h4 className="text-sm font-medium">添加证据关系</h4>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="正文版本 ID" name="document_version_id" />
            <Field label="引用选择器 ID" name="text_quote_selector_id" />
            <Field label="主体" name="subject" />
            <Field label="动作 / 谓词" name="predicate" />
            <div className="sm:col-span-2"><Field label="对象 / 陈述内容" name="object" /></div>
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="claim-relation">证据关系</Label>
              <Select onValueChange={(value) => setRelation(value as (typeof relations)[number])} value={relation}>
                <SelectTrigger id="claim-relation"><SelectValue /></SelectTrigger>
                <SelectContent>{relations.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent>
              </Select>
            </div>
          </div>
          <Button disabled={busy != null} type="submit">{busy === "evidence" ? <Loader2 className="animate-spin" /> : <Save />}保存证据关系</Button>
        </form>

        <form className="space-y-4 border-t border-border pt-6" onSubmit={submitGovernance}>
          <h4 className="text-sm font-medium">事件治理</h4>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-2 sm:col-span-2">
              <Label htmlFor="governance-action">动作</Label>
              <Select onValueChange={(value) => setAction(value as (typeof governanceActions)[number])} value={action}>
                <SelectTrigger id="governance-action"><SelectValue /></SelectTrigger>
                <SelectContent>{governanceActions.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}</SelectContent>
              </Select>
            </div>
            <Field label="成员决策 ID（移动 / 拆分）" name="membership_decision_id" required={false} />
            <Field label="内容家族 ID（移动 / 拆分）" name="content_family_id" required={false} />
            <Field label="成员版本（移动）" name="expected_member_version" required={false} />
            <Field label="目标微事件 ID（合并 / 移动）" name="target_micro_event_id" required={false} />
            <Field label="目标事件版本" name="expected_target_event_version" required={false} />
            <Field label="原因代码" name="reason_code" />
            <div className="sm:col-span-2"><Field label="备注（可选）" name="note" required={false} /></div>
          </div>
          <Button disabled={busy != null} type="submit" variant="outline">{busy === "governance" ? <Loader2 className="animate-spin" /> : <Save />}保存治理反馈</Button>
        </form>
      </CardContent>
    </Card>
  );
}

function Field({ label, name, required = true }: { label: string; name: string; required?: boolean }) {
  return <div className="space-y-2"><Label htmlFor={`micro-event-${name}`}>{label}</Label><Input id={`micro-event-${name}`} name={name} required={required} /></div>;
}
