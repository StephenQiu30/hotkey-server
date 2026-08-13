"use client";

import { type FormEvent, useState } from "react";
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

type AuthType = "none" | "api_key" | "oauth2" | "bearer";
type GoogleLocation = "global" | "us" | "eu";
type HackerNewsMode = "new" | "top" | "best";

const authLabels: Record<AuthType, string> = {
  none: "无需授权",
  api_key: "API Key",
  oauth2: "OAuth 2.0",
  bearer: "Bearer Token",
};

const GOOGLE_ENDPOINTS: Record<GoogleLocation, string> = {
  global: "https://discoveryengine.googleapis.com",
  us: "https://us-discoveryengine.googleapis.com",
  eu: "https://eu-discoveryengine.googleapis.com",
};

const sourceProfiles: Record<
  SourceType,
  {
    label: string;
    endpoint: string;
    endpointFixed: boolean;
    authType: AuthType;
    authFixed: boolean;
    enabledOnCreate: boolean;
    termsPolicyURL: string;
    termsFixed: boolean;
    requiresAttribution: boolean;
    requiresDeletionSync: boolean;
  }
> = {
  [SourceType.RSS]: {
    label: "RSS / Atom",
    endpoint: "",
    endpointFixed: false,
    authType: "none",
    authFixed: false,
    enabledOnCreate: true,
    termsPolicyURL: "",
    termsFixed: false,
    requiresAttribution: false,
    requiresDeletionSync: false,
  },
  [SourceType.HackerNews]: {
    label: "Hacker News",
    endpoint: "https://hacker-news.firebaseio.com/v0",
    endpointFixed: true,
    authType: "none",
    authFixed: true,
    enabledOnCreate: true,
    termsPolicyURL: "",
    termsFixed: false,
    requiresAttribution: false,
    requiresDeletionSync: false,
  },
  [SourceType.X]: {
    label: "X / Twitter",
    endpoint: "https://api.x.com/2/tweets/search/recent",
    endpointFixed: true,
    authType: "bearer",
    authFixed: true,
    enabledOnCreate: false,
    termsPolicyURL: "",
    termsFixed: false,
    requiresAttribution: false,
    requiresDeletionSync: false,
  },
  [SourceType.BingGrounding]: {
    label: "Bing Grounding",
    endpoint: "",
    endpointFixed: false,
    authType: "bearer",
    authFixed: true,
    enabledOnCreate: false,
    termsPolicyURL:
      "https://learn.microsoft.com/en-us/azure/ai-foundry/agents/how-to/tools/web-search?view=foundry",
    termsFixed: true,
    requiresAttribution: true,
    requiresDeletionSync: false,
  },
  [SourceType.Bilibili]: {
    label: "Bilibili",
    endpoint: "https://member.bilibili.com/arcopen/fn",
    endpointFixed: true,
    authType: "oauth2",
    authFixed: true,
    enabledOnCreate: false,
    termsPolicyURL: "https://openhome.bilibili.com/agreement/privacy-policy",
    termsFixed: true,
    requiresAttribution: true,
    requiresDeletionSync: true,
  },
  [SourceType.Weibo]: {
    label: "Weibo",
    endpoint: "https://open.weibo.com/cli/api",
    endpointFixed: true,
    authType: "bearer",
    authFixed: true,
    enabledOnCreate: false,
    termsPolicyURL:
      "https://open.weibo.com/wiki/%E5%BC%80%E5%8F%91%E8%80%85%E5%8D%8F%E8%AE%AE",
    termsFixed: true,
    requiresAttribution: true,
    requiresDeletionSync: true,
  },
  [SourceType.GoogleAgentSearch]: {
    label: "Google Agent Search",
    endpoint: GOOGLE_ENDPOINTS.global,
    endpointFixed: true,
    authType: "bearer",
    authFixed: true,
    enabledOnCreate: false,
    termsPolicyURL: "https://cloud.google.com/terms",
    termsFixed: true,
    requiresAttribution: true,
    requiresDeletionSync: false,
  },
};

type SourceForm = {
  sourceType: SourceType;
  name: string;
  endpoint: string;
  authType: AuthType;
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
  termsPolicyURL: string;
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
  sourceType: SourceType.RSS,
  name: "",
  endpoint: "",
  authType: "none",
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
  termsPolicyURL: "",
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

const validHTTPSURL = (value: string) => {
  try {
    return new URL(value).protocol === "https:";
  } catch {
    return false;
  }
};

const googleServingConfigPattern =
  /^projects\/[a-z][a-z0-9-]{4,28}[a-z0-9]\/locations\/(global|us|eu)\/collections\/default_collection\/dataStores\/[A-Za-z0-9_-]{1,63}\/servingConfigs\/[A-Za-z0-9_-]{1,63}$/;

type Props = {
  busy: boolean;
  onSubmit: (request: HotKeyAPI.CreateSourceRequest) => Promise<boolean>;
};

export function SourceConnectionDialog({ busy, onSubmit }: Props) {
  const [open, setOpen] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [form, setForm] = useState<SourceForm>(emptyForm);
  const profile = sourceProfiles[form.sourceType];
  const updateForm = (values: Partial<SourceForm>) =>
    setForm((current) => ({ ...current, ...values }));

  const selectSource = (sourceType: SourceType) => {
    const next = sourceProfiles[sourceType];
    setForm((current) => ({
      ...current,
      sourceType,
      endpoint: next.endpoint,
      authType: next.authType,
      credential: "",
      allowBodyStorage: true,
      requiresAttribution: next.requiresAttribution,
      requiresDeletionSync: next.requiresDeletionSync,
      maxPagesPerRun: sourceType === SourceType.BingGrounding ? 1 : current.maxPagesPerRun,
      termsPolicyURL: next.termsPolicyURL,
      groundingDataBoundaryApproved: false,
      bilibiliOpenID: "",
      googleLocation: "global",
      googleServingConfig: "",
      hackerNewsMode: "top",
      metricRefreshEnabled: false,
    }));
  };

  const credentialBytes = new TextEncoder().encode(form.credential).length;
  const credentialValid =
    form.authType === "none"
      ? form.credential.length === 0
      : Boolean(form.credential.trim()) && credentialBytes <= 16 * 1024;
  const valid = Boolean(
    form.name.trim() &&
      validHTTPSURL(form.endpoint) &&
      credentialValid &&
      (!form.termsPolicyURL || validHTTPSURL(form.termsPolicyURL)) &&
      Number.isInteger(form.contentRetentionDays) && form.contentRetentionDays >= 1 && form.contentRetentionDays <= 3650 &&
      Number.isInteger(form.metricsRetentionDays) && form.metricsRetentionDays >= 1 && form.metricsRetentionDays <= 3650 &&
      Number.isInteger(form.rateLimitPerMinute) && form.rateLimitPerMinute >= 1 && form.rateLimitPerMinute <= 600 &&
      Number.isInteger(form.requestTimeoutSeconds) && form.requestTimeoutSeconds >= 1 && form.requestTimeoutSeconds <= 120 &&
      Number.isInteger(form.maxPagesPerRun) && form.maxPagesPerRun >= 1 && form.maxPagesPerRun <= 20 &&
      (form.sourceType !== SourceType.BingGrounding || form.groundingDataBoundaryApproved) &&
      (form.sourceType !== SourceType.Bilibili || /^[A-Za-z0-9_-]{1,128}$/.test(form.bilibiliOpenID.trim())) &&
      (form.sourceType !== SourceType.GoogleAgentSearch ||
        (googleServingConfigPattern.test(form.googleServingConfig.trim()) &&
          form.googleServingConfig.includes(`/locations/${form.googleLocation}/`)))
  );

  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (!next) {
      setForm(emptyForm());
      setAdvancedOpen(false);
    }
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!valid) return;
    const created = await onSubmit({
      source_type: form.sourceType as HotKeyAPI.CreateSourceRequest["source_type"],
      name: form.name.trim(),
      endpoint: form.endpoint.trim(),
      auth_type: form.authType,
      ...(form.authType === "none" ? {} : { credential: form.credential }),
      enabled: profile.enabledOnCreate,
      terms_policy_url: form.termsPolicyURL.trim(),
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
        ...(form.sourceType === SourceType.BingGrounding
          ? { grounding_data_boundary_approved: form.groundingDataBoundaryApproved }
          : {}),
        ...(form.sourceType === SourceType.Bilibili
          ? { bilibili_open_id: form.bilibiliOpenID.trim() }
          : {}),
        ...(form.sourceType === SourceType.GoogleAgentSearch
          ? {
              google_location: form.googleLocation,
              google_serving_config: form.googleServingConfig.trim(),
            }
          : {}),
        ...(form.sourceType === SourceType.HackerNews
          ? { hacker_news_mode: form.hackerNewsMode }
          : {}),
        ...(form.sourceType === SourceType.X
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
          <DialogDescription>
            先完成连接所需信息；采集和保留策略将使用安全默认值。
          </DialogDescription>
        </DialogHeader>
        <form className="grid min-h-0 grid-rows-[minmax(0,1fr)_auto]" onSubmit={submit}>
          <div aria-label="来源连接配置" className="grid min-h-0 gap-5 overflow-y-auto px-6 py-5 sm:grid-cols-2" role="region" tabIndex={0}>
            <div>
              <Label htmlFor="source-type">来源类型</Label>
              <Select value={form.sourceType} onValueChange={(value) => selectSource(value as SourceType)}>
                <SelectTrigger id="source-type" className="mt-2 h-10">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(sourceProfiles).map(([value, item]) => (
                    <SelectItem key={value} value={value}>{item.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {profile.endpointFixed && (
                <p className="mt-2 text-xs text-muted-foreground">
                  官方接口 · {authLabels[form.authType]}
                </p>
              )}
            </div>
            <div>
              <Label htmlFor="source-name">名称</Label>
              <Input id="source-name" className="mt-2" value={form.name} onChange={(event) => updateForm({ name: event.target.value })} placeholder={`${profile.label} 热点`} />
            </div>
            {!profile.endpointFixed && <div className="sm:col-span-2">
              <Label htmlFor="source-endpoint">接口地址</Label>
              <Input id="source-endpoint" className="mono mt-2" value={form.endpoint} onChange={(event) => updateForm({ endpoint: event.target.value })} placeholder={form.sourceType === SourceType.BingGrounding ? "https://…/api/projects/…/toolboxes/…/versions/…/mcp" : "https://example.com/feed.xml"} />
              <p className="mt-2 text-xs text-muted-foreground">
                {form.sourceType === SourceType.RSS ? "填写 RSS 或 Atom Feed 的 HTTPS 地址。" : "填写已获授权的官方接口地址。"}
              </p>
            </div>}
            {form.authType !== "none" && <div className="sm:col-span-2">
              <Label htmlFor="source-credential">访问凭据</Label>
              <PasswordInput id="source-credential" className="mono mt-2" value={form.credential} onChange={(event) => updateForm({ credential: event.target.value })} autoComplete="new-password" maxLength={16 * 1024} aria-invalid={!credentialValid} placeholder="粘贴 API Key、Token 或开放平台凭据" />
              <p className="mt-2 text-xs text-muted-foreground">凭据由服务端加密保存，提交后不会回显。</p>
            </div>}
            {form.sourceType === SourceType.HackerNews && <div className="sm:col-span-2">
              <Label htmlFor="source-hn-mode">榜单模式</Label>
              <Select value={form.hackerNewsMode} onValueChange={(value) => updateForm({ hackerNewsMode: value as HackerNewsMode })}>
                <SelectTrigger id="source-hn-mode" className="mt-2 h-10"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="top">热门榜单</SelectItem>
                  <SelectItem value="best">最佳榜单</SelectItem>
                  <SelectItem value="new">最新项目</SelectItem>
                </SelectContent>
              </Select>
            </div>}
            {form.sourceType === SourceType.Bilibili && <div className="sm:col-span-2">
              <Label htmlFor="source-bilibili-open-id">授权账号 OpenID</Label>
              <Input id="source-bilibili-open-id" className="mono mt-2" value={form.bilibiliOpenID} onChange={(event) => updateForm({ bilibiliOpenID: event.target.value })} />
            </div>}
            {form.sourceType === SourceType.GoogleAgentSearch && <>
              <div><Label htmlFor="source-google-location">数据位置</Label><Select value={form.googleLocation} onValueChange={(value) => { const location = value as GoogleLocation; updateForm({ googleLocation: location, endpoint: GOOGLE_ENDPOINTS[location], googleServingConfig: "" }); }}><SelectTrigger id="source-google-location" className="mt-2 h-10"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="global">Global</SelectItem><SelectItem value="us">US</SelectItem><SelectItem value="eu">EU</SelectItem></SelectContent></Select></div>
              <div><Label htmlFor="source-google-serving-config">ServingConfig 资源名</Label><Input id="source-google-serving-config" className="mono mt-2" value={form.googleServingConfig} onChange={(event) => updateForm({ googleServingConfig: event.target.value })} /></div>
            </>}
            {form.sourceType === SourceType.BingGrounding && <Alert className="sm:col-span-2"><AlertDescription><label className="flex items-start gap-2"><Checkbox aria-label="确认 Grounding 数据边界与额外条款" checked={form.groundingDataBoundaryApproved} onCheckedChange={(checked) => updateForm({ groundingDataBoundaryApproved: checked === true })} /><span>我已确认 Foundry Web Search 的数据边界、额外条款和费用。</span></label></AlertDescription></Alert>}
            <div className="sm:col-span-2 border-t border-border pt-4">
              <Button
                type="button"
                variant="ghost"
                className="h-auto w-full justify-between px-0 py-2 hover:bg-transparent"
                aria-expanded={advancedOpen}
                aria-controls="source-advanced-settings"
                onClick={() => setAdvancedOpen((current) => !current)}
              >
                <span className="flex items-center gap-2"><SlidersHorizontal />高级设置</span>
                <ChevronDown className={`transition-transform ${advancedOpen ? "rotate-180" : ""}`} />
              </Button>
              <p className="text-xs text-muted-foreground">语言、地区、保留期限和请求边界将使用默认值，可按需调整。</p>
            </div>
            {advancedOpen && <div id="source-advanced-settings" className="grid gap-4 rounded-lg border border-border bg-muted/20 p-4 sm:col-span-2 sm:grid-cols-2">
              {!profile.authFixed && <div className="sm:col-span-2">
                <Label htmlFor="source-auth-type">授权方式</Label>
                <Select value={form.authType} onValueChange={(value) => updateForm({ authType: value as AuthType, credential: "" })}>
                  <SelectTrigger id="source-auth-type" className="mt-2 h-10"><SelectValue /></SelectTrigger>
                  <SelectContent><SelectItem value="none">无需授权</SelectItem><SelectItem value="api_key">API Key</SelectItem><SelectItem value="bearer">Bearer Token</SelectItem><SelectItem value="oauth2">OAuth 2.0</SelectItem></SelectContent>
                </Select>
              </div>}
              <div><Label htmlFor="source-languages">允许语言</Label><Input id="source-languages" className="mt-2" value={form.allowedLanguages} onChange={(event) => updateForm({ allowedLanguages: event.target.value })} placeholder="默认不限制" /></div>
              <div><Label htmlFor="source-regions">允许地区</Label><Input id="source-regions" className="mt-2" value={form.allowedRegions} onChange={(event) => updateForm({ allowedRegions: event.target.value })} placeholder="默认不限制" /></div>
              <div className="rounded-md border border-border bg-background p-4 sm:col-span-2">
                <p className="text-sm font-medium">证据保留边界</p>
                <div className="mt-3 grid gap-3 sm:grid-cols-3">
                  <label className="flex items-center gap-2 text-sm"><Checkbox checked={form.allowBodyStorage} disabled={profile.requiresAttribution} onCheckedChange={(checked) => updateForm({ allowBodyStorage: checked === true })} />保存正文/摘要</label>
                  <label className="flex items-center gap-2 text-sm"><Checkbox checked={form.requiresAttribution} disabled={profile.requiresAttribution} onCheckedChange={(checked) => updateForm({ requiresAttribution: checked === true })} />需要归属标记</label>
                  <label className="flex items-center gap-2 text-sm"><Checkbox checked={form.requiresDeletionSync} disabled={profile.requiresDeletionSync} onCheckedChange={(checked) => updateForm({ requiresDeletionSync: checked === true })} />同步删除</label>
                </div>
              </div>
              {(["contentRetentionDays", "metricsRetentionDays", "rateLimitPerMinute", "requestTimeoutSeconds", "maxPagesPerRun"] as const).map((key) => {
                const labels = { contentRetentionDays: "内容保留天数", metricsRetentionDays: "指标保留天数", rateLimitPerMinute: "每分钟请求上限", requestTimeoutSeconds: "请求超时秒数", maxPagesPerRun: "单次最大页数" };
                return <div key={key}><Label htmlFor={`source-${key}`}>{labels[key]}</Label><Input id={`source-${key}`} className="mono mt-2" type="number" value={form[key]} onChange={(event) => updateForm({ [key]: Number(event.target.value) })} disabled={form.sourceType === SourceType.BingGrounding && key === "maxPagesPerRun"} /></div>;
              })}
              {form.sourceType === SourceType.X && <div className="rounded-md border border-border bg-background p-4 sm:col-span-2">
                <label className="flex items-start gap-3 text-sm font-medium"><Checkbox aria-label="启用 X 持续指标刷新" checked={form.metricRefreshEnabled} onCheckedChange={(checked) => updateForm({ metricRefreshEnabled: checked === true })} /><span>启用 X 持续指标刷新<span className="mt-1 block text-xs font-normal text-muted-foreground">默认关闭；仅刷新观察期内已发现的 Post，并受来源预算限制。</span></span></label>
                {form.metricRefreshEnabled && <div className="mt-4 grid gap-4 sm:grid-cols-2">
                  {(["metricRefreshIntervalMinutes", "metricRefreshObservationHours", "metricRefreshMaxPostsPerRun", "metricRefreshDailyRequestBudget"] as const).map((key) => {
                    const labels = { metricRefreshIntervalMinutes: "刷新间隔（分钟）", metricRefreshObservationHours: "持续观察期（小时）", metricRefreshMaxPostsPerRun: "单轮最多 Post", metricRefreshDailyRequestBudget: "每日批次预算" };
                    return <div key={key}><Label htmlFor={`source-${key}`}>{labels[key]}</Label><Input id={`source-${key}`} className="mono mt-2" type="number" value={form[key]} onChange={(event) => updateForm({ [key]: Number(event.target.value) })} /></div>;
                  })}
                </div>}
              </div>}
              <div className="sm:col-span-2"><Label htmlFor="source-terms-url">条款与政策地址</Label><Input id="source-terms-url" className="mt-2" value={form.termsPolicyURL} readOnly={profile.termsFixed} onChange={(event) => updateForm({ termsPolicyURL: event.target.value })} placeholder="可选" /></div>
            </div>}
          </div>
          <DialogFooter className="border-t border-border px-6 py-4"><Button type="button" variant="outline" onClick={() => changeOpen(false)}>取消</Button><Button type="submit" disabled={busy || !valid}>{busy && <Loader2 className="animate-spin" />}创建连接</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
