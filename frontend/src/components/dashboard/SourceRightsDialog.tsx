"use client";

import { useCallback, useRef, useState } from "react";
import { Loader2, ShieldCheck } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
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
import { Textarea } from "@/components/ui/textarea";
import {
  getSourceEndpointsIdRightsDecisionBatches,
  getSourceEndpointsIdRightsPolicies,
  postSourceEndpointsIdRightsDecisionBatches,
  postSourceEndpointsIdRightsPolicies,
} from "@/services/hotkey/hotkey-server/sourceRights";

type DecisionChoice = "omit" | "allow" | "deny" | "unknown";

const rightsActions = [
  { action: "fetch", label: "抓取" },
  { action: "store_raw", label: "保存原始证据" },
  { action: "store_derived", label: "保存派生结果" },
  { action: "display_private", label: "私有展示" },
  { action: "redistribute", label: "再分发" },
  { action: "quote", label: "引用" },
  { action: "embed_local", label: "本地分析索引" },
  { action: "send_external_model", label: "发送外部模型" },
  { action: "retain", label: "保留" },
] as const;

const emptyDecisions = () =>
  Object.fromEntries(
    rightsActions.map(({ action }) => [action, action === "fetch" ? "allow" : "omit"]),
  ) as Record<(typeof rightsActions)[number]["action"], DecisionChoice>;

const decisionLabels: Record<Exclude<DecisionChoice, "omit">, string> = {
  allow: "允许",
  deny: "拒绝",
  unknown: "未知",
};

type Attempt = {
  decisionKey: string;
  effectiveFrom: string;
  policyKey: string;
};

type Props = {
  actorUserID: number;
  source: HotKeyAPI.SourceReadResponse;
};

function idempotencyKey(prefix: string, sourceID: number) {
  const suffix =
    typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `${prefix}-${sourceID}-${suffix}`;
}

function validURL(value: string, optional = false) {
  const candidate = value.trim();
  if (!candidate) return optional;
  try {
    const parsed = new URL(candidate);
    return parsed.protocol === "https:" || parsed.protocol === "http:";
  } catch {
    return false;
  }
}

function latestPolicy(items: HotKeyAPI.RightsPolicyResponseDTO[]) {
  return [...items].sort(
    (left, right) =>
      (right.revision ?? 0) - (left.revision ?? 0) ||
      (right.id ?? 0) - (left.id ?? 0),
  )[0];
}

function latestBatch(
  items: HotKeyAPI.RightsDecisionBatchResponseDTO[],
  policyID?: number,
) {
  return items
    .filter((item) => policyID == null || item.policy_id === policyID)
    .toSorted((left, right) => (right.id ?? 0) - (left.id ?? 0))[0];
}

export function SourceRightsDialog({ actorUserID, source }: Props) {
  const sourceID = source.id ?? 0;
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [loadError, setLoadError] = useState<string>();
  const [saveError, setSaveError] = useState<string>();
  const [success, setSuccess] = useState<string>();
  const [policies, setPolicies] = useState<HotKeyAPI.RightsPolicyResponseDTO[]>([]);
  const [batches, setBatches] = useState<HotKeyAPI.RightsDecisionBatchResponseDTO[]>([]);
  const [pendingPolicy, setPendingPolicy] = useState<HotKeyAPI.RightsPolicyResponseDTO>();
  const [basisSummary, setBasisSummary] = useState("");
  const [termsURL, setTermsURL] = useState(source.terms_policy_url ?? "");
  const [licenseURI, setLicenseURI] = useState("");
  const [retentionDays, setRetentionDays] = useState(
    source.config?.content_retention_days ?? 30,
  );
  const [decisions, setDecisions] = useState(emptyDecisions);
  const attempt = useRef<Attempt | undefined>(undefined);

  const loadHistory = useCallback(async () => {
    if (sourceID <= 0) return;
    setLoading(true);
    setLoadError(undefined);
    try {
      const [policyResult, batchResult] = await Promise.all([
        getSourceEndpointsIdRightsPolicies({ id: sourceID, limit: 50 }),
        getSourceEndpointsIdRightsDecisionBatches({ id: sourceID, limit: 50 }),
      ]);
      const nextPolicies = policyResult.data?.items ?? [];
      const nextBatches = batchResult.data?.items ?? [];
      const currentPolicy = latestPolicy(nextPolicies);
      const currentBatch = latestBatch(nextBatches, currentPolicy?.id);
      setPolicies(nextPolicies);
      setBatches(nextBatches);
      setPendingPolicy(
        currentPolicy?.id != null &&
          !nextBatches.some((batch) => batch.policy_id === currentPolicy.id)
          ? currentPolicy
          : undefined,
      );
      if (currentPolicy) {
        setBasisSummary((value) => value || currentPolicy.basis_summary || "");
        setTermsURL((value) => value || currentPolicy.terms_url || "");
        setLicenseURI((value) => value || currentPolicy.license_uri || "");
      }
      if (currentBatch?.decisions?.length) {
        const restored = emptyDecisions();
        for (const decision of currentBatch.decisions) {
          if (
            decision.action &&
            decision.action in restored &&
            (decision.decision === "allow" ||
              decision.decision === "deny" ||
              decision.decision === "unknown")
          ) {
            restored[decision.action as keyof typeof restored] = decision.decision;
          }
          if (decision.action === "retain" && decision.retention_days) {
            setRetentionDays(decision.retention_days);
          }
        }
        setDecisions(restored);
      }
    } catch (reason) {
      setLoadError(reason instanceof Error ? reason.message : "使用权历史加载失败");
    } finally {
      setLoading(false);
    }
  }, [sourceID]);

  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (next) {
      setSuccess(undefined);
      setLoadError(undefined);
      setSaveError(undefined);
      void loadHistory();
    }
  };

  const declaredActions = rightsActions.filter(
    ({ action }) => decisions[action] !== "omit",
  );
  const valid =
    sourceID > 0 &&
    actorUserID > 0 &&
    basisSummary.trim().length > 0 &&
    new TextEncoder().encode(basisSummary.trim()).length <= 1024 &&
    validURL(termsURL) &&
    validURL(licenseURI, true) &&
    declaredActions.length > 0 &&
    (decisions.retain === "omit" ||
      (Number.isInteger(retentionDays) &&
        retentionDays >= 1 &&
        retentionDays <= 3650));

  const save = async () => {
    if (!valid || submitting) return;
    setSubmitting(true);
    setSaveError(undefined);
    setSuccess(undefined);
    if (!attempt.current) {
      attempt.current = {
        policyKey: idempotencyKey("rights-policy", sourceID),
        decisionKey: idempotencyKey("rights-decisions", sourceID),
        effectiveFrom: new Date().toISOString(),
      };
    }
    try {
      let policy = pendingPolicy;
      if (!policy) {
        const revision =
          policies.reduce((highest, item) => Math.max(highest, item.revision ?? 0), 0) + 1;
        const result = await postSourceEndpointsIdRightsPolicies(
          { id: sourceID },
          {
            approved_by_user_id: actorUserID,
            basis_summary: basisSummary.trim(),
            effective_from: attempt.current.effectiveFrom,
            ...(licenseURI.trim() ? { license_uri: licenseURI.trim() } : {}),
            priority: 300,
            revision,
            scope_subject: String(sourceID),
            scope_type: "source_endpoint",
            terms_url: termsURL.trim(),
          },
          {
            headers: {
              "Content-Type": "application/json",
              "Idempotency-Key": attempt.current.policyKey,
            },
          },
        );
        policy = result.data?.policy;
        if (!policy?.id || !policy.version || !policy.policy_hash) {
          throw new Error("服务端未返回完整的不可变使用权策略");
        }
        setPendingPolicy(policy);
      }
      if (!policy.id || !policy.version || !policy.policy_hash) {
        throw new Error("待记录的使用权策略不完整");
      }
      const decisionEffectiveFrom =
        policy.effective_from && policy.effective_from > attempt.current.effectiveFrom
          ? policy.effective_from
          : attempt.current.effectiveFrom;
      await postSourceEndpointsIdRightsDecisionBatches(
        { id: sourceID },
        {
          decisions: declaredActions.map(({ action }) => ({
            action,
            decision: decisions[action] as Exclude<DecisionChoice, "omit">,
            effective_from: decisionEffectiveFrom,
            evaluated_at: attempt.current!.effectiveFrom,
            evaluator: "admin_rights_workspace",
            reason_codes: ["admin_terms_reviewed"],
            ...(action === "retain" ? { retention_days: retentionDays } : {}),
          })),
          expected_policy_version: policy.version,
          input_digest: policy.policy_hash,
          policy_id: policy.id,
          subject_key: String(sourceID),
          subject_type: "source_endpoint",
        },
        {
          headers: {
            "Content-Type": "application/json",
            "Idempotency-Key": attempt.current.decisionKey,
            "If-Match": `"v${policy.version}"`,
          },
        },
      );
      attempt.current = undefined;
      setPendingPolicy(undefined);
      setSuccess("使用权策略已保存");
      await loadHistory();
    } catch (reason) {
      setSaveError(reason instanceof Error ? reason.message : "使用权策略保存失败");
    } finally {
      setSubmitting(false);
    }
  };

  const currentPolicy = latestPolicy(policies);
  const currentBatch = latestBatch(batches, currentPolicy?.id);

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button
          aria-label={`管理 ${source.name ?? `来源 #${sourceID}`} 的使用权`}
          className="gap-1.5"
          disabled={sourceID <= 0 || source.deleted}
          size="sm"
          variant="outline"
        >
          <ShieldCheck />
          使用权
        </Button>
      </DialogTrigger>
      <DialogContent className="grid max-h-[90vh] grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:max-w-3xl">
        <DialogHeader className="bg-secondary/30 px-6 py-5">
          <DialogTitle>{source.name ?? `来源 #${sourceID}`} 使用权</DialogTitle>
          <DialogDescription>
            策略和动作决策只追加、不覆盖。只有明确允许的动作可以执行；未声明、未知或拒绝都会失败关闭。
          </DialogDescription>
        </DialogHeader>
        <div className="min-h-0 space-y-5 overflow-y-auto px-6 py-5">
          {loading ? (
            <div aria-label="正在加载使用权历史" className="flex min-h-24 items-center justify-center" role="status">
              <Loader2 className="animate-spin text-muted-foreground" />
            </div>
          ) : (
            <>
              <section aria-label="当前使用权历史" className="rounded-lg border p-4">
                {currentPolicy ? (
                  <div className="space-y-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <p className="text-sm font-medium">修订 {currentPolicy.revision ?? "—"}</p>
                      <Badge variant={currentPolicy.approved_by_user_id ? "secondary" : "outline"}>
                        {currentPolicy.approved_by_user_id ? "已审批" : "未审批"}
                      </Badge>
                    </div>
                    <p className="text-sm leading-6 text-muted-foreground">{currentPolicy.basis_summary}</p>
                    {currentPolicy.terms_url ? (
                      <a className="text-xs underline underline-offset-4" href={currentPolicy.terms_url} rel="noreferrer" target="_blank">
                        查看策略依据
                      </a>
                    ) : null}
                    <div className="flex flex-wrap gap-2">
                      {(currentBatch?.decisions ?? []).map((decision) => (
                        <Badge key={decision.id ?? `${decision.action}-${decision.decision}`} variant="outline">
                          {rightsActions.find((item) => item.action === decision.action)?.label ?? decision.action}：
                          {decisionLabels[decision.decision as keyof typeof decisionLabels] ?? decision.decision}
                        </Badge>
                      ))}
                    </div>
                    {pendingPolicy ? (
                      <p className="text-xs text-amber-700 dark:text-amber-300">该策略尚无完整决策批次，本次保存将继续完成它。</p>
                    ) : null}
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground">尚未登记使用权策略</p>
                )}
              </section>

              <div className="space-y-2">
                <Label htmlFor={`rights-basis-${sourceID}`}>授权依据</Label>
                <Textarea
                  disabled={Boolean(pendingPolicy)}
                  id={`rights-basis-${sourceID}`}
                  maxLength={1024}
                  onChange={(event) => setBasisSummary(event.target.value)}
                  placeholder="说明官方条款、许可范围、用途限制和未授权边界"
                  value={basisSummary}
                />
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor={`rights-terms-${sourceID}`}>条款地址</Label>
                  <Input disabled={Boolean(pendingPolicy)} id={`rights-terms-${sourceID}`} onChange={(event) => setTermsURL(event.target.value)} placeholder="https://…" value={termsURL} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor={`rights-license-${sourceID}`}>许可地址（可选）</Label>
                  <Input disabled={Boolean(pendingPolicy)} id={`rights-license-${sourceID}`} onChange={(event) => setLicenseURI(event.target.value)} placeholder="https://…" value={licenseURI} />
                </div>
              </div>

              <fieldset className="space-y-3">
                <legend className="text-sm font-medium">动作决策</legend>
                <div className="grid gap-3 sm:grid-cols-2">
                  {rightsActions.map(({ action, label }) => (
                    <div className="grid grid-cols-[minmax(0,1fr)_140px] items-center gap-3 rounded-md border px-3 py-2" key={action}>
                      <Label htmlFor={`rights-action-${sourceID}-${action}`}>{label}</Label>
                      <Select
                        onValueChange={(value) =>
                          setDecisions((current) => ({ ...current, [action]: value as DecisionChoice }))
                        }
                        value={decisions[action]}
                      >
                        <SelectTrigger aria-label={`${label}决策`} id={`rights-action-${sourceID}-${action}`}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="omit">未声明</SelectItem>
                          <SelectItem value="allow">允许</SelectItem>
                          <SelectItem value="deny">拒绝</SelectItem>
                          <SelectItem value="unknown">未知</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  ))}
                </div>
              </fieldset>
              {decisions.retain !== "omit" ? (
                <div className="max-w-xs space-y-2">
                  <Label htmlFor={`rights-retention-${sourceID}`}>保留天数</Label>
                  <Input
                    id={`rights-retention-${sourceID}`}
                    max={3650}
                    min={1}
                    onChange={(event) => setRetentionDays(Number(event.target.value))}
                    type="number"
                    value={retentionDays}
                  />
                </div>
              ) : null}
            </>
          )}
          {loadError ? (
            <Alert variant="destructive">
              <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
                <span>{loadError}</span>
                <Button
                  disabled={loading || submitting}
                  onClick={() => void loadHistory()}
                  size="sm"
                  type="button"
                  variant="outline"
                >
                  重新加载使用权历史
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {saveError ? <Alert variant="destructive"><AlertDescription>{saveError}</AlertDescription></Alert> : null}
          {success ? <Alert><AlertDescription>{success}</AlertDescription></Alert> : null}
        </div>
        <DialogFooter className="bg-secondary/25 px-6 py-4">
          <Button disabled={submitting} onClick={() => setOpen(false)} type="button" variant="outline">关闭</Button>
          <Button disabled={loading || submitting || !valid} onClick={() => void save()} type="button">
            {submitting ? <Loader2 className="animate-spin" /> : null}
            保存使用权策略
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
