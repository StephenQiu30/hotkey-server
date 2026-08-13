"use client";

import { type FormEvent, useEffect, useMemo, useState } from "react";
import { ChevronDown, Loader2, Plus, SlidersHorizontal } from "lucide-react";
import PasswordInput from "@/components/auth/PasswordInput";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { SourceType } from "@/lib/domainEnums";
import { getSourcePresets } from "@/services/hotkey/hotkey-server/sources";

type GoogleLocation = "global" | "us" | "eu";
type HackerNewsMode = "new" | "top" | "best";

type SourceForm = {
  presetID: string;
  presetValues: Record<string, string>;
  name: string;
  credential: string;
  allowBodyStorage: boolean;
  requiresAttribution: boolean;
  requiresDeletionSync: boolean;
  allowedLanguages: string;
  allowedRegions: string;
  contentRetentionDays: number;
  metricsRetentionDays: number;
  rateLimitPerMinute: number;
  requestTimeoutSeconds: number;
  maxPagesPerRun: number;
  groundingDataBoundaryApproved: boolean;
  bilibiliOpenID: string;
  googleLocation: GoogleLocation;
  googleServingConfig: string;
  hackerNewsMode: HackerNewsMode;
  metricRefreshEnabled: boolean;
  metricRefreshIntervalMinutes: number;
  metricRefreshObservationHours: number;
  metricRefreshMaxPostsPerRun: number;
  metricRefreshDailyRequestBudget: number;
};

const emptyForm = (): SourceForm => ({
  presetID: "",
  presetValues: {},
  name: "",
  credential: "",
  allowBodyStorage: true,
  requiresAttribution: false,
  requiresDeletionSync: false,
  allowedLanguages: "",
  allowedRegions: "",
  contentRetentionDays: 30,
  metricsRetentionDays: 30,
  rateLimitPerMinute: 60,
  requestTimeoutSeconds: 30,
  maxPagesPerRun: 1,
  groundingDataBoundaryApproved: false,
  bilibiliOpenID: "",
  googleLocation: "global",
  googleServingConfig: "",
  hackerNewsMode: "top",
  metricRefreshEnabled: false,
  metricRefreshIntervalMinutes: 60,
  metricRefreshObservationHours: 48,
  metricRefreshMaxPostsPerRun: 100,
  metricRefreshDailyRequestBudget: 24,
});

const splitValues = (value: string) =>
  value.split(",").map((item) => item.trim()).filter(Boolean);

const googleServingConfigPattern =
  /^projects\/[a-z][a-z0-9-]{4,28}[a-z0-9]\/locations\/(global|us|eu)\/collections\/default_collection\/dataStores\/[A-Za-z0-9_-]{1,63}\/servingConfigs\/[A-Za-z0-9_-]{1,63}$/;

type Props = {
  busy: boolean;
  onSubmit: (request: HotKeyAPI.CreateSourceRequest) => Promise<boolean>;
};

export function SourceConnectionDialog({ busy, onSubmit }: Props) {
  const [open, setOpen] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [catalog, setCatalog] = useState<HotKeyAPI.SourcePresetResponse[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [catalogError, setCatalogError] = useState<string>();
  const [form, setForm] = useState<SourceForm>(emptyForm);
  const preset = useMemo(
    () => catalog.find((item) => item.id === form.presetID),
    [catalog, form.presetID],
  );
  const sourceType = preset?.source_type as SourceType | undefined;
  const updateForm = (values: Partial<SourceForm>) =>
    setForm((current) => ({ ...current, ...values }));

  useEffect(() => {
    if (!open || catalog.length) return;
    let active = true;
    setCatalogLoading(true);
    setCatalogError(undefined);
    void getSourcePresets()
      .then((result) => {
        if (!active) return;
        const items = result.data?.items ?? [];
        setCatalog(items);
        setForm((current) => ({ ...current, presetID: items[0]?.id ?? "" }));
      })
      .catch((reason) => {
        if (active) setCatalogError(reason instanceof Error ? reason.message : "来源预设加载失败");
      })
      .finally(() => active && setCatalogLoading(false));
    return () => { active = false; };
  }, [catalog.length, open]);

  const selectPreset = (presetID: string) => {
    setForm((current) => ({
      ...current,
      presetID,
      presetValues: {},
      credential: "",
      allowBodyStorage: true,
      requiresAttribution: false,
      requiresDeletionSync: false,
      maxPagesPerRun: 1,
      groundingDataBoundaryApproved: false,
      bilibiliOpenID: "",
      googleLocation: "global",
      googleServingConfig: "",
      hackerNewsMode: "top",
      metricRefreshEnabled: false,
    }));
  };

  const credentialBytes = new TextEncoder().encode(form.credential).length;
  const credentialValid = preset?.credential_required
    ? Boolean(form.credential.trim()) && credentialBytes <= 16 * 1024
    : form.credential.length === 0;
  const presetValuesValid = Boolean(preset?.inputs?.every((input) => {
    const value = form.presetValues[input.key ?? ""] ?? "";
    return (!input.required || Boolean(value.trim())) &&
      (!input.max_length || [...value].length <= input.max_length);
  }));
  const valid = Boolean(
    preset && form.name.trim() && presetValuesValid && credentialValid &&
      Number.isInteger(form.contentRetentionDays) && form.contentRetentionDays >= 1 && form.contentRetentionDays <= 3650 &&
      Number.isInteger(form.metricsRetentionDays) && form.metricsRetentionDays >= 1 && form.metricsRetentionDays <= 3650 &&
      Number.isInteger(form.rateLimitPerMinute) && form.rateLimitPerMinute >= 1 && form.rateLimitPerMinute <= 600 &&
      Number.isInteger(form.requestTimeoutSeconds) && form.requestTimeoutSeconds >= 1 && form.requestTimeoutSeconds <= 120 &&
      Number.isInteger(form.maxPagesPerRun) && form.maxPagesPerRun >= 1 && form.maxPagesPerRun <= 20 &&
      (sourceType !== SourceType.BingGrounding || form.groundingDataBoundaryApproved) &&
      (sourceType !== SourceType.Bilibili || /^[A-Za-z0-9_-]{1,128}$/.test(form.bilibiliOpenID.trim())) &&
      (sourceType !== SourceType.GoogleAgentSearch ||
        (googleServingConfigPattern.test(form.googleServingConfig.trim()) &&
          form.googleServingConfig.includes(`/locations/${form.googleLocation}/`)))
  );

  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (!next) {
      const firstPresetID = catalog[0]?.id ?? "";
      setForm({ ...emptyForm(), presetID: firstPresetID });
      setAdvancedOpen(false);
    }
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!valid || !preset?.id) return;
    const created = await onSubmit({
      preset_id: preset.id,
      preset_values: (preset.inputs ?? []).map((input) => ({
        key: input.key ?? "",
        value: form.presetValues[input.key ?? ""]?.trim() ?? "",
      })),
      name: form.name.trim(),
      ...(preset.credential_required ? { credential: form.credential } : {}),
      config: {
        allow_body_storage: form.allowBodyStorage,
        requires_attribution: form.requiresAttribution,
        requires_deletion_sync: form.requiresDeletionSync,
        content_retention_days: form.contentRetentionDays,
        metrics_retention_days: form.metricsRetentionDays,
        allowed_languages: splitValues(form.allowedLanguages),
        allowed_regions: splitValues(form.allowedRegions),
        rate_limit_per_minute: form.rateLimitPerMinute,
        request_timeout_seconds: form.requestTimeoutSeconds,
        max_pages_per_run: form.maxPagesPerRun,
        ...(sourceType === SourceType.BingGrounding
          ? { grounding_data_boundary_approved: form.groundingDataBoundaryApproved }
          : {}),
        ...(sourceType === SourceType.Bilibili
          ? { bilibili_open_id: form.bilibiliOpenID.trim() }
          : {}),
        ...(sourceType === SourceType.GoogleAgentSearch
          ? { google_location: form.googleLocation, google_serving_config: form.googleServingConfig.trim() }
          : {}),
        ...(sourceType === SourceType.HackerNews
          ? { hacker_news_mode: form.hackerNewsMode }
          : {}),
        ...(sourceType === SourceType.X
          ? {
              x_metric_refresh_enabled: form.metricRefreshEnabled,
              x_metric_refresh_interval_minutes: form.metricRefreshIntervalMinutes,
              x_metric_refresh_observation_hours: form.metricRefreshObservationHours,
              x_metric_refresh_max_posts_per_run: form.metricRefreshMaxPostsPerRun,
              x_metric_refresh_daily_request_budget: form.metricRefreshDailyRequestBudget,
            }
          : {}),
      },
    });
    if (created) changeOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button className="gap-2"><Plus />新增来源</Button>
      </DialogTrigger>
      <DialogContent className="grid max-h-[90vh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogHeader className="border-b border-border px-6 py-5">
          <DialogTitle>新增来源连接</DialogTitle>
          <DialogDescription>选择由后端维护的来源预设；官方地址、授权方式和固定策略由服务端统一配置。</DialogDescription>
        </DialogHeader>
        <form className="grid min-h-0 grid-rows-[minmax(0,1fr)_auto]" onSubmit={submit}>
          <div aria-label="来源连接配置" className="grid min-h-0 gap-5 overflow-y-auto px-6 py-5 sm:grid-cols-2" role="region" tabIndex={0}>
            {catalogError && <Alert variant="destructive" className="sm:col-span-2"><AlertDescription>{catalogError}</AlertDescription></Alert>}
            <div>
              <Label htmlFor="source-preset">接入方式</Label>
              <Select value={form.presetID} disabled={catalogLoading || !catalog.length} onValueChange={selectPreset}>
                <SelectTrigger id="source-preset" className="mt-2 h-10"><SelectValue placeholder={catalogLoading ? "加载后端预设…" : "选择来源"} /></SelectTrigger>
                <SelectContent>
                  {catalog.map((item) => <SelectItem key={item.id} value={item.id ?? ""}>{item.label}</SelectItem>)}
                </SelectContent>
              </Select>
              {preset && <p className="mt-2 text-xs text-muted-foreground">{preset.auth_label} · {preset.cost === "free" ? "免费" : preset.cost === "paid" ? "官方付费" : "需要平台凭据"}</p>}
            </div>
            <div>
              <Label htmlFor="source-name">名称</Label>
              <Input id="source-name" className="mt-2" value={form.name} onChange={(event) => updateForm({ name: event.target.value })} placeholder={`${preset?.label?.replace(/（.+）$/, "") ?? "来源"} 热点`} />
            </div>
            {preset?.description && <p className="text-xs text-muted-foreground sm:col-span-2">{preset.description}</p>}
            {preset?.inputs?.map((input) => <div key={input.key} className={preset.inputs?.length === 1 ? "sm:col-span-2" : ""}>
              <Label htmlFor={`source-preset-${input.key}`}>{input.label}</Label>
              <Input
                id={`source-preset-${input.key}`}
                className="mono mt-2"
                value={form.presetValues[input.key ?? ""] ?? ""}
                onChange={(event) => updateForm({ presetValues: { ...form.presetValues, [input.key ?? ""]: event.target.value } })}
                maxLength={input.max_length}
                placeholder={input.placeholder}
              />
            </div>)}
            {preset?.cost === "paid" && <Alert className="sm:col-span-2"><AlertDescription>X 官方 API 按量付费；仅有 Bearer Token 不代表账户已有可用额度。个人免费方案请优先选择公开 Feed。</AlertDescription></Alert>}
            {preset?.credential_required && <div className="sm:col-span-2">
              <Label htmlFor="source-credential">访问凭据</Label>
              <PasswordInput id="source-credential" className="mono mt-2" value={form.credential} onChange={(event) => updateForm({ credential: event.target.value })} autoComplete="new-password" maxLength={16 * 1024} aria-invalid={!credentialValid} placeholder={preset.auth_label} />
              <p className="mt-2 text-xs text-muted-foreground">凭据由服务端加密保存，提交后不会回显。</p>
            </div>}
            {sourceType === SourceType.HackerNews && <div className="sm:col-span-2">
              <Label htmlFor="source-hn-mode">榜单模式</Label>
              <Select value={form.hackerNewsMode} onValueChange={(value) => updateForm({ hackerNewsMode: value as HackerNewsMode })}><SelectTrigger id="source-hn-mode" className="mt-2 h-10"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="top">热门榜单</SelectItem><SelectItem value="best">最佳榜单</SelectItem><SelectItem value="new">最新项目</SelectItem></SelectContent></Select>
            </div>}
            {sourceType === SourceType.Bilibili && <div className="sm:col-span-2"><Label htmlFor="source-bilibili-open-id">授权账号 OpenID</Label><Input id="source-bilibili-open-id" className="mono mt-2" value={form.bilibiliOpenID} onChange={(event) => updateForm({ bilibiliOpenID: event.target.value })} /></div>}
            {sourceType === SourceType.GoogleAgentSearch && <>
              <div><Label htmlFor="source-google-location">数据位置</Label><Select value={form.googleLocation} onValueChange={(value) => updateForm({ googleLocation: value as GoogleLocation, googleServingConfig: "" })}><SelectTrigger id="source-google-location" className="mt-2 h-10"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="global">Global</SelectItem><SelectItem value="us">US</SelectItem><SelectItem value="eu">EU</SelectItem></SelectContent></Select></div>
              <div><Label htmlFor="source-google-serving-config">ServingConfig 资源名</Label><Input id="source-google-serving-config" className="mono mt-2" value={form.googleServingConfig} onChange={(event) => updateForm({ googleServingConfig: event.target.value })} /></div>
            </>}
            {sourceType === SourceType.BingGrounding && <Alert className="sm:col-span-2"><AlertDescription><label className="flex items-start gap-2"><Checkbox aria-label="确认 Grounding 数据边界与额外条款" checked={form.groundingDataBoundaryApproved} onCheckedChange={(checked) => updateForm({ groundingDataBoundaryApproved: checked === true })} /><span>我已确认 Foundry Web Search 的数据边界、额外条款和费用。</span></label></AlertDescription></Alert>}
            <div className="sm:col-span-2 border-t border-border pt-4">
              <Button type="button" variant="ghost" className="h-auto w-full justify-between px-0 py-2 hover:bg-transparent" aria-expanded={advancedOpen} aria-controls="source-advanced-settings" onClick={() => setAdvancedOpen((current) => !current)}><span className="flex items-center gap-2"><SlidersHorizontal />高级设置</span><ChevronDown className={`transition-transform ${advancedOpen ? "rotate-180" : ""}`} /></Button>
              <p className="text-xs text-muted-foreground">语言、地区、保留期限和请求边界将使用默认值，可按需调整。</p>
            </div>
            {advancedOpen && <div id="source-advanced-settings" className="grid gap-4 rounded-lg border border-border bg-muted/20 p-4 sm:col-span-2 sm:grid-cols-2">
              <div><Label htmlFor="source-languages">允许语言</Label><Input id="source-languages" className="mt-2" value={form.allowedLanguages} onChange={(event) => updateForm({ allowedLanguages: event.target.value })} placeholder="默认不限制" /></div>
              <div><Label htmlFor="source-regions">允许地区</Label><Input id="source-regions" className="mt-2" value={form.allowedRegions} onChange={(event) => updateForm({ allowedRegions: event.target.value })} placeholder="默认不限制" /></div>
              <div className="rounded-md border border-border bg-background p-4 sm:col-span-2"><p className="text-sm font-medium">证据保留边界</p><div className="mt-3 grid gap-3 sm:grid-cols-3"><label className="flex items-center gap-2 text-sm"><Checkbox checked={form.allowBodyStorage} onCheckedChange={(checked) => updateForm({ allowBodyStorage: checked === true })} />保存正文/摘要</label><label className="flex items-center gap-2 text-sm"><Checkbox checked={form.requiresAttribution} onCheckedChange={(checked) => updateForm({ requiresAttribution: checked === true })} />需要归属标记</label><label className="flex items-center gap-2 text-sm"><Checkbox checked={form.requiresDeletionSync} onCheckedChange={(checked) => updateForm({ requiresDeletionSync: checked === true })} />同步删除</label></div></div>
              {(["contentRetentionDays", "metricsRetentionDays", "rateLimitPerMinute", "requestTimeoutSeconds", "maxPagesPerRun"] as const).map((key) => {
                const labels = { contentRetentionDays: "内容保留天数", metricsRetentionDays: "指标保留天数", rateLimitPerMinute: "每分钟请求上限", requestTimeoutSeconds: "请求超时秒数", maxPagesPerRun: "单次最大页数" };
                return <div key={key}><Label htmlFor={`source-${key}`}>{labels[key]}</Label><Input id={`source-${key}`} className="mono mt-2" type="number" value={form[key]} onChange={(event) => updateForm({ [key]: Number(event.target.value) })} disabled={sourceType === SourceType.BingGrounding && key === "maxPagesPerRun"} /></div>;
              })}
              {sourceType === SourceType.X && <div className="rounded-md border border-border bg-background p-4 sm:col-span-2"><label className="flex items-start gap-3 text-sm font-medium"><Checkbox aria-label="启用 X 持续指标刷新" checked={form.metricRefreshEnabled} onCheckedChange={(checked) => updateForm({ metricRefreshEnabled: checked === true })} /><span>启用 X 持续指标刷新<span className="mt-1 block text-xs font-normal text-muted-foreground">默认关闭；刷新仍受官方 API 额度限制。</span></span></label>{form.metricRefreshEnabled && <div className="mt-4 grid gap-4 sm:grid-cols-2">{(["metricRefreshIntervalMinutes", "metricRefreshObservationHours", "metricRefreshMaxPostsPerRun", "metricRefreshDailyRequestBudget"] as const).map((key) => { const labels = { metricRefreshIntervalMinutes: "刷新间隔（分钟）", metricRefreshObservationHours: "持续观察期（小时）", metricRefreshMaxPostsPerRun: "单轮最多 Post", metricRefreshDailyRequestBudget: "每日批次预算" }; return <div key={key}><Label htmlFor={`source-${key}`}>{labels[key]}</Label><Input id={`source-${key}`} className="mono mt-2" type="number" value={form[key]} onChange={(event) => updateForm({ [key]: Number(event.target.value) })} /></div>; })}</div>}</div>}
            </div>}
          </div>
          <DialogFooter className="border-t border-border px-6 py-4"><Button type="button" variant="outline" onClick={() => changeOpen(false)}>取消</Button><Button type="submit" disabled={busy || catalogLoading || !valid}>{(busy || catalogLoading) && <Loader2 className="animate-spin" />}创建连接</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
