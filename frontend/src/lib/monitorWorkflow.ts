import { HotKeyAPIError } from "@/lib/request";
import {
  getMonitorsIdVersions,
  postMonitorsIdPublish,
  putMonitorsIdDraft,
} from "@/services/hotkey/hotkey-server/monitors";
import {
  getMonitorsIdDraft,
  getMonitorsIdDraftPreviewRunsRunId,
  postMonitorsIdDraftPreviewRuns,
  putMonitorsIdDraftIntent,
} from "@/services/hotkey/hotkey-server/monitorIntent";

export type SimpleMonitorForm = {
  name: string;
  query: string;
  interval: string;
  alertEmailEnabled: boolean;
  sourceIds: number[];
};

export type MonitorScanState = {
  queued?: boolean;
  items: HotKeyAPI.MonitorScanResponse[];
};

export type SimpleMonitorFields = {
  name: string;
  query: string;
  source_connection_ids: number[];
  collection_interval_seconds: number;
  alert_email_enabled: boolean;
};

const intentPreviewProfile = "hybrid-preview-v1";
const intentPreviewSampleLimit = 25;
const maximumIntentPreviewPolls = 40;

function intentETag(version: number) {
  return `"v${version}"`;
}

function simpleDraftRequest(
  fields: SimpleMonitorFields,
  expectedMonitorVersion: number,
  expectedDraftVersion: number | null
) {
  return {
    expected_monitor_version: expectedMonitorVersion,
    expected_draft_version: expectedDraftVersion,
    name: fields.name,
    description: `监控 ${fields.query}`,
    config: {
      timezone: "Asia/Shanghai",
      languages: ["zh", "en"],
      collection_interval_seconds: fields.collection_interval_seconds,
      relevance_threshold: 60,
      event_threshold: 0,
      alert_min_heat: 70,
      alert_min_momentum: 55,
      alert_min_breadth: 25,
      alert_warning_threshold: 75,
      alert_critical_threshold: 90,
      alert_cooldown_minutes: 60,
      alert_email_enabled: fields.alert_email_enabled,
      alert_email_min_severity: "warning",
      retention_days: 30,
    },
    rules: [
      {
        rule_type: "keyword",
        operator: "contains",
        value: fields.query,
        weight: 100,
        priority: 1,
        enabled: true,
      },
    ],
    sources: fields.source_connection_ids.map((sourceConnectionID, index) => ({
      source_connection_id: sourceConnectionID,
      priority: index + 1,
      enabled: true,
      query_override: "",
    })),
  } as unknown as HotKeyAPI.ReplaceDraftRequest;
}

function intentPreviewIdempotencyKey(
  monitorID: number,
  intentResourceVersion: number
) {
  const entropy =
    globalThis.crypto?.randomUUID?.() ??
    `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  return `simple-monitor-preview-${monitorID}-${intentResourceVersion}-${entropy}`.slice(
    0,
    128
  );
}

async function currentIntentResourceVersion(monitorID: number) {
  try {
    const response = await getMonitorsIdDraft({ id: monitorID });
    const version = response.data?.resource_version;
    if (version == null || version <= 0) {
      throw new Error("监控意图版本无效");
    }
    return version;
  } catch (reason) {
    if (reason instanceof HotKeyAPIError && reason.status === 404) return 0;
    throw reason;
  }
}

async function waitForIntentPreview(monitorID: number, runID: number) {
  for (let attempt = 0; attempt < maximumIntentPreviewPolls; attempt += 1) {
    const response = await getMonitorsIdDraftPreviewRunsRunId({
      id: monitorID,
      run_id: runID,
    });
    const status = response.data?.status;
    if (status === "succeeded") return;
    if (status === "failed" || status === "invalidated") {
      throw new Error(response.data?.failure_code || "监控意图预览失败");
    }
    if (status !== "queued" && status !== "running") {
      throw new Error("监控意图预览状态无效");
    }
    await new Promise((resolve) => globalThis.setTimeout(resolve, 500));
  }
  throw new Error("监控意图预览超时，请稍后重试");
}

export async function compileAndPublishSimpleMonitor(
  monitorID: number,
  monitorVersion: number,
  fields: SimpleMonitorFields
) {
  const existingHistory = await getMonitorsIdVersions({
    id: monitorID,
    limit: 100,
  });
  const expectedDraftVersion =
    existingHistory.data?.items?.find(
      (configuration) => configuration.state === "draft"
    )?.version ?? null;
  const drafted = await putMonitorsIdDraft(
    { id: monitorID },
    simpleDraftRequest(fields, monitorVersion, expectedDraftVersion)
  );
  const draftedMonitorVersion = drafted.data?.version;
  if (draftedMonitorVersion == null || draftedMonitorVersion <= monitorVersion) {
    throw new Error("监控草稿版本无效");
  }

  const currentResourceVersion =
    expectedDraftVersion == null
      ? 0
      : await currentIntentResourceVersion(monitorID);
  const intent = await putMonitorsIdDraftIntent(
    { id: monitorID },
    {
      expected_resource_version: currentResourceVersion,
      objective: fields.query,
      clauses: [{ operator: "should", field: "term", value: fields.query }],
      entities: [],
      examples: [],
    },
    {
      headers:
        currentResourceVersion === 0
          ? { "If-None-Match": "*" }
          : { "If-Match": intentETag(currentResourceVersion) },
    }
  );
  const intentResourceVersion = intent.data?.resource_version;
  if (intentResourceVersion == null || intentResourceVersion <= 0) {
    throw new Error("监控意图保存失败");
  }

  const preview = await postMonitorsIdDraftPreviewRuns(
    { id: monitorID },
    {
      expected_resource_version: intentResourceVersion,
      evaluator_profile: intentPreviewProfile,
      sample_limit: intentPreviewSampleLimit,
    },
    {
      headers: {
        "If-Match": intentETag(intentResourceVersion),
        "Idempotency-Key": intentPreviewIdempotencyKey(
          monitorID,
          intentResourceVersion
        ),
      },
    }
  );
  const previewRunID = preview.data?.run_id;
  if (previewRunID == null || previewRunID <= 0) {
    throw new Error("监控意图预览未排队");
  }
  await waitForIntentPreview(monitorID, previewRunID);

  const history = await getMonitorsIdVersions({ id: monitorID, limit: 100 });
  const draftVersion = history.data?.items?.find(
    (configuration) => configuration.state === "draft"
  )?.version;
  if (draftVersion == null || draftVersion <= 0) {
    throw new Error("监控发布草稿不存在");
  }
  await postMonitorsIdPublish(
    { id: monitorID },
    {
      expected_monitor_version: draftedMonitorVersion,
      expected_draft_version: draftVersion,
    }
  );
}

export const emptyMonitorForm = (): SimpleMonitorForm => ({
  name: "",
  query: "",
  interval: "1800",
  alertEmailEnabled: true,
  sourceIds: [],
});

export const monitorScanStatusLabels: Readonly<Record<string, string>> = {
  queued: "已排队",
  running: "扫描中",
  succeeded: "成功",
  partial: "部分成功",
  failed: "失败",
  cancelled: "已取消",
};

export function monitorQuery(monitor: HotKeyAPI.MonitorResponse) {
  return monitor.query ?? monitor.name ?? "";
}

export function monitorIntervalLabel(seconds: number | undefined) {
  if (!seconds) return "—";
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`;
  return `${seconds / 60} 分钟`;
}

export function formatMonitorTime(value: string | undefined) {
  if (!value) return "尚未运行";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "尚未运行";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}
