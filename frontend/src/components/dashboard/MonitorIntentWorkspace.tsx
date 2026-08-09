"use client";

import { useCallback, useEffect, useState } from "react";
import { Loader2, RefreshCw, Save, Search, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { ExpansionCandidateReview } from "@/components/dashboard/ExpansionCandidateReview";
import { IntentPreviewPanel } from "@/components/dashboard/IntentPreviewPanel";
import { MonitorIntentEditor } from "@/components/dashboard/MonitorIntentEditor";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  emptyMonitorIntentDraft,
  monitorIntentDraftFromResponse,
  monitorIntentDraftRequest,
  validateMonitorIntentDraft,
  type MonitorIntentDraftForm,
} from "@/lib/monitorIntentDraft";
import { HotKeyAPIError } from "@/lib/request";
import {
  getMonitorsIdDraft,
  getMonitorsIdDraftExpansionRunsRunId,
  getMonitorsIdDraftPreviewRunsRunId,
  postMonitorsIdDraftExpansionCandidatesCandidateIdDecision,
  postMonitorsIdDraftExpansionRuns,
  postMonitorsIdDraftPreviewRuns,
  putMonitorsIdDraftIntent,
} from "@/services/hotkey/hotkey-server/monitorIntent";

const terminalRunStatuses = new Set(["succeeded", "failed", "invalidated"]);
const intentExpansionProfile = "monitor-intent-expansion-v1" as const;
let actionSequence = 0;

type IntentRunStatus = "queued" | "running" | "succeeded" | "failed" | "invalidated";

function runStatus(value?: string): IntentRunStatus | undefined {
  switch (value) {
    case "queued":
    case "running":
    case "succeeded":
    case "failed":
    case "invalidated":
      return value;
    default:
      return undefined;
  }
}

function idempotencyKey(prefix: string, monitorID: number, resourceVersion: number): string {
  actionSequence += 1;
  return `${prefix}-${monitorID}-${resourceVersion}-${Date.now().toString(36)}-${actionSequence.toString(36)}`;
}

function intentConditionHeaders(resourceVersion: number): Record<string, string> {
  return resourceVersion > 0
    ? { "Content-Type": "application/json", "If-Match": `"v${resourceVersion}"` }
    : { "Content-Type": "application/json", "If-None-Match": "*" };
}

function intentActionHeaders(prefix: string, monitorID: number, resourceVersion: number) {
  return {
    "Content-Type": "application/json",
    "If-Match": `"v${resourceVersion}"`,
    "Idempotency-Key": idempotencyKey(prefix, monitorID, resourceVersion),
  };
}

type MonitorIntentWorkspaceProps = {
  canAdmin: boolean;
  monitorID: number;
  pollIntervalMs?: number;
};

export function MonitorIntentWorkspace({
  canAdmin,
  monitorID,
  pollIntervalMs = 1500,
}: MonitorIntentWorkspaceProps) {
  const [draft, setDraft] = useState<HotKeyAPI.IntentDraftResponseDTO>();
  const [form, setForm] = useState<MonitorIntentDraftForm>(emptyMonitorIntentDraft);
  const [dirty, setDirty] = useState(false);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string>();
  const [saving, setSaving] = useState(false);
  const [expansionRunID, setExpansionRunID] = useState<number>();
  const [expansionRun, setExpansionRun] =
    useState<HotKeyAPI.IntentExpansionRunStatusResponseDTO>();
  const [previewProfile, setPreviewProfile] = useState("hybrid-recall-v1");
  const [previewRunID, setPreviewRunID] = useState<number>();
  const [previewRun, setPreviewRun] =
    useState<HotKeyAPI.IntentPreviewRunStatusResponseDTO>();
  const [busyCandidateID, setBusyCandidateID] = useState<string>();

  const applyDraftProjection = useCallback((next: HotKeyAPI.IntentDraftResponseDTO) => {
    setDraft(next);
    setForm(monitorIntentDraftFromResponse(next));
    setDirty(false);
  }, []);

  const refreshDraft = useCallback(
    async (showLoading: boolean) => {
      if (showLoading) setLoading(true);
      setLoadError(undefined);
      try {
        const result = await getMonitorsIdDraft({ id: monitorID });
        if (!result.data) throw new Error("语义意图响应为空");
        applyDraftProjection(result.data);
      } catch (reason) {
        if (reason instanceof HotKeyAPIError && reason.status === 404) {
          setDraft(undefined);
          setForm(emptyMonitorIntentDraft());
          setDirty(false);
        } else {
          setLoadError(reason instanceof Error ? reason.message : "语义意图加载失败");
        }
      } finally {
        if (showLoading) setLoading(false);
      }
    },
    [applyDraftProjection, monitorID],
  );

  useEffect(() => {
    void refreshDraft(true);
  }, [refreshDraft]);

  useEffect(() => {
    if (!expansionRunID) return;
    let cancelled = false;
    let reportedFailure = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = async () => {
      try {
        const result = await getMonitorsIdDraftExpansionRunsRunId({
          id: monitorID,
          run_id: expansionRunID,
        });
        if (cancelled || !result.data) return;
        reportedFailure = false;
        setExpansionRun(result.data);
        if (result.data.status === "succeeded") {
          await refreshDraft(false);
          return;
        }
        if (!terminalRunStatuses.has(result.data.status ?? "")) {
          timer = setTimeout(() => void poll(), pollIntervalMs);
        }
      } catch (reason) {
        if (!cancelled) {
          if (!reportedFailure) {
            toast.error(reason instanceof Error ? reason.message : "扩展任务状态读取失败");
            reportedFailure = true;
          }
          timer = setTimeout(() => void poll(), pollIntervalMs);
        }
      }
    };
    void poll();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [expansionRunID, monitorID, pollIntervalMs, refreshDraft]);

  useEffect(() => {
    if (!previewRunID) return;
    let cancelled = false;
    let reportedFailure = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = async () => {
      try {
        const result = await getMonitorsIdDraftPreviewRunsRunId({
          id: monitorID,
          run_id: previewRunID,
        });
        if (cancelled || !result.data) return;
        reportedFailure = false;
        setPreviewRun(result.data);
        if (!terminalRunStatuses.has(result.data.status ?? "")) {
          timer = setTimeout(() => void poll(), pollIntervalMs);
        }
      } catch (reason) {
        if (!cancelled) {
          if (!reportedFailure) {
            toast.error(reason instanceof Error ? reason.message : "预览任务状态读取失败");
            reportedFailure = true;
          }
          timer = setTimeout(() => void poll(), pollIntervalMs);
        }
      }
    };
    void poll();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [monitorID, pollIntervalMs, previewRunID]);

  const resourceVersion = draft?.resource_version ?? 0;
  const expansionActive = expansionRun?.status === "queued" || expansionRun?.status === "running";
  const actionBlocked = resourceVersion <= 0 || dirty || saving || expansionActive;
  const candidates =
    (draft?.candidates ?? []).length > 0
      ? draft?.candidates ?? []
      : expansionRun?.candidates ?? [];

  const changeForm = (next: MonitorIntentDraftForm) => {
    setForm(next);
    setDirty(true);
  };

  const save = async () => {
    const validation = validateMonitorIntentDraft(form);
    if (validation) {
      toast.error(validation);
      return;
    }
    setSaving(true);
    try {
      const result = await putMonitorsIdDraftIntent(
        { id: monitorID },
        monitorIntentDraftRequest(form, resourceVersion),
        { headers: intentConditionHeaders(resourceVersion) },
      );
      if (!result.data) throw new Error("语义意图保存响应为空");
      applyDraftProjection(result.data);
      setExpansionRun(undefined);
      setPreviewRun(undefined);
      toast.success(resourceVersion > 0 ? "语义意图已保存" : "语义意图已初始化");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "语义意图保存失败");
    } finally {
      setSaving(false);
    }
  };

  const submitExpansion = async () => {
    if (actionBlocked || !canAdmin) return;
    try {
      const result = await postMonitorsIdDraftExpansionRuns(
        { id: monitorID },
        {
          expected_resource_version: resourceVersion,
          expansion_profile: intentExpansionProfile,
        },
        { headers: intentActionHeaders("intent-expansion", monitorID, resourceVersion) },
      );
      const accepted = result.data as HotKeyAPI.IntentRunAcceptedResponseDTO | undefined;
      if (!accepted?.run_id) throw new Error("扩展任务响应缺少 run_id");
      setExpansionRunID(accepted.run_id);
      setExpansionRun({ run_id: accepted.run_id, status: runStatus(accepted.status) });
      toast.success(accepted.reused ? "已复用同一扩展任务" : "扩展任务已提交");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "扩展任务提交失败");
    }
  };

  const submitPreview = async () => {
    if (actionBlocked) return;
    const profile = previewProfile.trim();
    if (!profile) return toast.error("请填写预览评估配置");
    try {
      const result = await postMonitorsIdDraftPreviewRuns(
        { id: monitorID },
        { expected_resource_version: resourceVersion, evaluator_profile: profile, sample_limit: 20 },
        { headers: intentActionHeaders("intent-preview", monitorID, resourceVersion) },
      );
      const accepted = result.data as HotKeyAPI.IntentRunAcceptedResponseDTO | undefined;
      if (!accepted?.run_id) throw new Error("预览任务响应缺少 run_id");
      setPreviewRunID(accepted.run_id);
      setPreviewRun({ run_id: accepted.run_id, status: runStatus(accepted.status) });
      toast.success(accepted.reused ? "已复用同一预览任务" : "预览任务已提交");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "预览任务提交失败");
    }
  };

  const reviewCandidate = async (
    candidate: HotKeyAPI.IntentExpansionCandidateResponseDTO,
    decision: "approved" | "rejected",
  ) => {
    if (!candidate.id || actionBlocked || !canAdmin) return;
    setBusyCandidateID(candidate.id);
    try {
      const result = await postMonitorsIdDraftExpansionCandidatesCandidateIdDecision(
        { id: monitorID, candidate_id: candidate.id },
        { expected_resource_version: resourceVersion, decision, note: "" },
        { headers: intentActionHeaders("intent-candidate", monitorID, resourceVersion) },
      );
      if (!result.data) throw new Error("候选审批响应为空");
      applyDraftProjection(result.data);
      toast.success(decision === "approved" ? "扩展候选已批准" : "扩展候选已拒绝");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "候选审批失败");
    } finally {
      setBusyCandidateID(undefined);
    }
  };

  if (loading) {
    return (
      <div className="flex min-h-72 items-center justify-center" aria-label="加载语义意图">
        <Loader2 className="animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (loadError) {
    return (
      <Card className="items-center p-8 text-center" role="alert">
        <p className="font-medium">语义意图加载失败</p>
        <p className="text-sm text-muted-foreground">{loadError}</p>
        <Button onClick={() => void refreshDraft(true)} type="button" variant="outline">
          <RefreshCw />
          重试
        </Button>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <Card className="flex-row flex-wrap items-center justify-between gap-4 p-5">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-lg font-semibold">语义监控意图</h1>
            <Badge variant="outline">
              {resourceVersion > 0 ? `资源版本 v${resourceVersion}` : "尚未初始化"}
            </Badge>
            {dirty ? <Badge variant="secondary">有未保存修改</Badge> : null}
          </div>
          <p className="mt-2 text-sm text-muted-foreground">
            意图、扩展建议和预览都绑定精确草稿版本；未审批建议不会进入采集或召回。
          </p>
        </div>
        <Button disabled={saving || expansionActive || !dirty} onClick={() => void save()} type="button">
          {saving ? <Loader2 className="animate-spin" /> : <Save />}
          保存语义意图
        </Button>
      </Card>

      <MonitorIntentEditor disabled={saving || expansionActive} form={form} onChange={changeForm} />

      <Card className="gap-4 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">AI 扩展建议</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              结果保留实际模型、提示版本、输入摘要、理由和风险；它只是待审批的相关性建议。
            </p>
          </div>
          {canAdmin ? (
            <div className="flex w-full flex-col gap-2 sm:w-auto sm:min-w-72">
              <Label htmlFor="intent-expansion-profile">扩词规范版本</Label>
              <div className="flex gap-2">
                <Input
                  id="intent-expansion-profile"
                  readOnly
                  value={intentExpansionProfile}
                />
                <Button
                  disabled={actionBlocked || expansionRun?.status === "queued" || expansionRun?.status === "running"}
                  onClick={() => void submitExpansion()}
                  type="button"
                >
                  <Sparkles />
                  生成扩展候选
                </Button>
              </div>
            </div>
          ) : null}
        </div>
        {actionBlocked && dirty ? (
          <p className="text-xs text-muted-foreground">请先保存当前修改，再运行版本绑定的任务。</p>
        ) : null}
        {expansionRun?.status === "queued" || expansionRun?.status === "running" ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
            <Loader2 className="h-4 w-4 animate-spin" />
            扩展任务正在{expansionRun.status === "queued" ? "排队" : "运行"}
          </p>
        ) : null}
        {expansionRun?.status === "failed" || expansionRun?.status === "invalidated" ? (
          <p className="text-sm text-destructive" role="alert">
            扩展任务{expansionRun.status === "invalidated" ? "因草稿变化已失效" : "失败"}
            {expansionRun.failure_code ? `：${expansionRun.failure_code}` : ""}
          </p>
        ) : null}
        <ExpansionCandidateReview
          busyCandidateID={busyCandidateID}
          canAdmin={canAdmin}
          candidates={candidates}
          onReview={reviewCandidate}
        />
      </Card>

      <Card className="gap-4 p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">历史样本相关性预览</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              展示各召回通道的名次与原始信号。不同通道的分值不可直接比较。
            </p>
          </div>
          <div className="flex w-full flex-col gap-2 sm:w-auto sm:min-w-72">
            <Label htmlFor="intent-preview-profile">预览评估配置</Label>
            <div className="flex gap-2">
              <Input
                disabled={actionBlocked}
                id="intent-preview-profile"
                onChange={(event) => setPreviewProfile(event.target.value)}
                value={previewProfile}
              />
              <Button
                disabled={actionBlocked || previewRun?.status === "queued" || previewRun?.status === "running"}
                onClick={() => void submitPreview()}
                type="button"
                variant="outline"
              >
                <Search />
                运行历史样本预览
              </Button>
            </div>
          </div>
        </div>
        {previewRun?.status === "failed" || previewRun?.status === "invalidated" ? (
          <p className="text-sm text-destructive" role="alert">
            预览任务{previewRun.status === "invalidated" ? "因草稿变化已失效" : "失败"}
            {previewRun.failure_code ? `：${previewRun.failure_code}` : ""}
          </p>
        ) : null}
        <IntentPreviewPanel preview={previewRun?.preview} status={previewRun?.status} />
      </Card>
    </div>
  );
}
