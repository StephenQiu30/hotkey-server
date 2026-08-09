"use client";

import { type FormEvent, useState } from "react";
import { Loader2, Plus, TriangleAlert } from "lucide-react";
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
import PasswordInput from "@/components/auth/PasswordInput";

const HACKER_NEWS_ENDPOINT = "https://hacker-news.firebaseio.com/v0";
const X_RECENT_SEARCH_ENDPOINT = "https://api.x.com/2/tweets/search/recent";
const FOUNDRY_WEB_SEARCH_POLICY_URL =
  "https://learn.microsoft.com/en-us/azure/ai-foundry/agents/how-to/tools/web-search?view=foundry";
const BILIBILI_OPEN_ENDPOINT = "https://member.bilibili.com/arcopen/fn";
const BILIBILI_PRIVACY_POLICY_URL =
  "https://openhome.bilibili.com/agreement/privacy-policy";
const WEIBO_CLI_API_ENDPOINT = "https://open.weibo.com/cli/api";
const WEIBO_DEVELOPER_TERMS_URL =
  "https://open.weibo.com/wiki/%E5%BC%80%E5%8F%91%E8%80%85%E5%8D%8F%E8%AE%AE";
const GOOGLE_AGENT_SEARCH_ENDPOINTS = {
  global: "https://discoveryengine.googleapis.com",
  us: "https://us-discoveryengine.googleapis.com",
  eu: "https://eu-discoveryengine.googleapis.com",
} as const;
const GOOGLE_CLOUD_TERMS_URL = "https://cloud.google.com/terms";
const googleServingConfigPattern =
  /^projects\/[a-z][a-z0-9-]{4,28}[a-z0-9]\/locations\/(global|us|eu)\/collections\/default_collection\/dataStores\/[A-Za-z0-9_-]{1,63}\/servingConfigs\/[A-Za-z0-9_-]{1,63}$/;

type AuthType = "none" | "api_key" | "oauth2" | "bearer";
type GoogleLocation = keyof typeof GOOGLE_AGENT_SEARCH_ENDPOINTS;
type HackerNewsMode = "new" | "top" | "best";

type SourceForm = {
  allowBodyStorage: boolean;
  allowedLanguages: string;
  allowedRegions: string;
  authType: AuthType;
  contentRetentionDays: number;
  credential: string;
  endpoint: string;
  maxPagesPerRun: number;
  metricsRetentionDays: number;
  name: string;
  rateLimitPerMinute: number;
  requestTimeoutSeconds: number;
  requiresAttribution: boolean;
  requiresDeletionSync: boolean;
  groundingDataBoundaryApproved: boolean;
  bilibiliOpenID: string;
  googleLocation: GoogleLocation;
  googleServingConfig: string;
  hackerNewsMode: HackerNewsMode;
  sourceType: SourceType;
  termsPolicyURL: string;
};

type ComplianceKey = keyof Pick<
  SourceForm,
  "allowBodyStorage" | "requiresAttribution" | "requiresDeletionSync"
>;
type NumericKey = keyof Pick<
  SourceForm,
  | "contentRetentionDays"
  | "metricsRetentionDays"
  | "rateLimitPerMinute"
  | "requestTimeoutSeconds"
  | "maxPagesPerRun"
>;

const complianceOptions: ReadonlyArray<{
  key: ComplianceKey;
  label: string;
}> = [
  {
    key: "allowBodyStorage",
    label: "保存来源正文/摘要用于归档预览",
  },
  { key: "requiresAttribution", label: "需要来源归属标记" },
  { key: "requiresDeletionSync", label: "需要同步删除" },
];

const numericOptions: ReadonlyArray<{
  key: NumericKey;
  label: string;
  min: number;
  max: number;
}> = [
  { key: "contentRetentionDays", label: "内容保留天数", min: 1, max: 3650 },
  { key: "metricsRetentionDays", label: "指标保留天数", min: 1, max: 3650 },
  { key: "rateLimitPerMinute", label: "每分钟请求上限", min: 1, max: 600 },
  { key: "requestTimeoutSeconds", label: "请求超时秒数", min: 1, max: 120 },
  { key: "maxPagesPerRun", label: "单次最大页数", min: 1, max: 20 },
];

const emptyForm = (): SourceForm => ({
  allowBodyStorage: true,
  allowedLanguages: "",
  allowedRegions: "",
  authType: "none",
  contentRetentionDays: 30,
  credential: "",
  endpoint: "",
  maxPagesPerRun: 1,
  metricsRetentionDays: 30,
  name: "",
  rateLimitPerMinute: 60,
  requestTimeoutSeconds: 30,
  requiresAttribution: false,
  requiresDeletionSync: false,
  groundingDataBoundaryApproved: false,
  bilibiliOpenID: "",
  googleLocation: "global",
  googleServingConfig: "",
  hackerNewsMode: "top",
  sourceType: SourceType.RSS,
  termsPolicyURL: "",
});

const splitValues = (value: string) =>
  value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);

const validHTTPSURL = (value: string) => {
  if (!value.trim()) return true;
  try {
    return new URL(value).protocol === "https:";
  } catch {
    return false;
  }
};

type Props = {
  busy: boolean;
  onSubmit: (request: HotKeyAPI.CreateSourceRequest) => Promise<boolean>;
};

export function SourceConnectionDialog({ busy, onSubmit }: Props) {
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<SourceForm>(emptyForm);

  const credentialValid =
    form.authType === "none"
      ? !form.credential
      : Boolean(form.credential.trim()) &&
        new TextEncoder().encode(form.credential).length <= 16 * 1024;
  const valid = Boolean(
    form.name.trim() &&
      form.endpoint.trim() &&
      credentialValid &&
      validHTTPSURL(form.termsPolicyURL) &&
      (form.sourceType !== SourceType.BingGrounding ||
        (form.groundingDataBoundaryApproved &&
          form.allowBodyStorage &&
          form.requiresAttribution &&
          form.maxPagesPerRun === 1 &&
          Boolean(form.termsPolicyURL.trim()))) &&
      (form.sourceType !== SourceType.Bilibili ||
        /^[A-Za-z0-9_-]{1,128}$/.test(form.bilibiliOpenID.trim())) &&
      (form.sourceType !== SourceType.GoogleAgentSearch ||
        (googleServingConfigPattern.test(form.googleServingConfig.trim()) &&
          form.googleServingConfig.includes(
            `/locations/${form.googleLocation}/`
          )))
  );

  const updateForm = (values: Partial<SourceForm>) => {
    setForm((current) => ({ ...current, ...values }));
  };

  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (!next) setForm(emptyForm());
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!valid) return;

    const created = await onSubmit({
      auth_type: form.authType,
      ...(form.authType === "none" ? {} : { credential: form.credential }),
      config: {
        allow_body_storage: form.allowBodyStorage,
        allowed_languages: splitValues(form.allowedLanguages),
        allowed_regions: splitValues(form.allowedRegions),
        content_retention_days: form.contentRetentionDays,
        max_pages_per_run: form.maxPagesPerRun,
        metrics_retention_days: form.metricsRetentionDays,
        rate_limit_per_minute: form.rateLimitPerMinute,
        request_timeout_seconds: form.requestTimeoutSeconds,
        requires_attribution: form.requiresAttribution,
        requires_deletion_sync: form.requiresDeletionSync,
        grounding_data_boundary_approved: form.groundingDataBoundaryApproved,
        bilibili_open_id: form.bilibiliOpenID.trim(),
        google_location: form.googleLocation,
        google_serving_config: form.googleServingConfig.trim(),
        hacker_news_mode:
          form.sourceType === SourceType.HackerNews
            ? form.hackerNewsMode
            : "new",
      },
      enabled:
        form.sourceType !== SourceType.X &&
        form.sourceType !== SourceType.BingGrounding &&
        form.sourceType !== SourceType.Bilibili &&
        form.sourceType !== SourceType.Weibo &&
        form.sourceType !== SourceType.GoogleAgentSearch,
      endpoint: form.endpoint.trim(),
      name: form.name.trim(),
      source_type: form.sourceType,
      terms_policy_url: form.termsPolicyURL.trim(),
    });
    if (created) changeOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button className="gap-2">
          <Plus />
          新增来源
        </Button>
      </DialogTrigger>
      <DialogContent className="grid h-[90vh] max-h-[90vh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:h-auto sm:max-w-2xl">
        <DialogHeader className="border-b border-border px-6 py-5">
          <DialogTitle>新增来源连接</DialogTitle>
          <DialogDescription>
            只连接官方 API、RSS / Atom 或已获书面授权的 Feed；访问凭据由服务端加密保存，提交后不再回显。
          </DialogDescription>
        </DialogHeader>
        <form
          className="grid min-h-0 grid-rows-[minmax(0,1fr)_auto]"
          onSubmit={submit}
        >
          <div
            aria-label="来源连接配置"
            className="grid min-h-0 gap-4 overflow-y-auto px-6 py-5 sm:grid-cols-2"
            role="region"
            tabIndex={0}
          >
            <div className="sm:col-span-2">
              <Label htmlFor="source-name">名称</Label>
              <Input
                id="source-name"
                className="mt-2"
                value={form.name}
                onChange={(event) => updateForm({ name: event.target.value })}
                placeholder="OpenAI 官方博客"
              />
            </div>
            <div>
              <Label htmlFor="source-type">来源类型</Label>
              <Select
                value={form.sourceType}
                onValueChange={(value) =>
                  updateForm({
                    sourceType: value as SourceType,
                    endpoint:
                      value === SourceType.HackerNews
                        ? HACKER_NEWS_ENDPOINT
                        : value === SourceType.X
                        ? X_RECENT_SEARCH_ENDPOINT
                        : value === SourceType.Bilibili
                        ? BILIBILI_OPEN_ENDPOINT
                        : value === SourceType.Weibo
                        ? WEIBO_CLI_API_ENDPOINT
                        : value === SourceType.GoogleAgentSearch
                        ? GOOGLE_AGENT_SEARCH_ENDPOINTS.global
                        : "",
                    authType:
                      value === SourceType.X ||
                      value === SourceType.BingGrounding
                        ? "bearer"
                        : value === SourceType.Bilibili
                        ? "oauth2"
                        : value === SourceType.Weibo
                        ? "bearer"
                        : value === SourceType.GoogleAgentSearch
                        ? "bearer"
                        : "none",
                    credential: "",
                    allowBodyStorage:
                      value === SourceType.BingGrounding ||
                      value === SourceType.Bilibili ||
                      value === SourceType.Weibo ||
                      value === SourceType.GoogleAgentSearch
                        ? true
                        : form.allowBodyStorage,
                    requiresAttribution:
                      value === SourceType.BingGrounding ||
                      value === SourceType.Bilibili ||
                      value === SourceType.Weibo ||
                      value === SourceType.GoogleAgentSearch
                        ? true
                        : form.requiresAttribution,
                    requiresDeletionSync:
                      value === SourceType.Bilibili ||
                      value === SourceType.Weibo
                        ? true
                        : value === SourceType.GoogleAgentSearch
                        ? false
                        : form.requiresDeletionSync,
                    maxPagesPerRun:
                      value === SourceType.BingGrounding
                        ? 1
                        : form.maxPagesPerRun,
                    groundingDataBoundaryApproved: false,
                    bilibiliOpenID: "",
                    googleLocation: "global",
                    googleServingConfig: "",
                    hackerNewsMode: "top",
                    termsPolicyURL:
                      value === SourceType.BingGrounding
                        ? FOUNDRY_WEB_SEARCH_POLICY_URL
                        : value === SourceType.Bilibili
                        ? BILIBILI_PRIVACY_POLICY_URL
                        : value === SourceType.Weibo
                        ? WEIBO_DEVELOPER_TERMS_URL
                        : value === SourceType.GoogleAgentSearch
                        ? GOOGLE_CLOUD_TERMS_URL
                        : "",
                  })
                }
              >
                <SelectTrigger id="source-type" className="mt-2">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={SourceType.RSS}>RSS / Atom</SelectItem>
                  <SelectItem value={SourceType.HackerNews}>
                    Hacker News
                  </SelectItem>
                  <SelectItem value={SourceType.X}>X Recent Search</SelectItem>
                  <SelectItem value={SourceType.BingGrounding}>
                    Microsoft Foundry Web Search
                  </SelectItem>
                  <SelectItem value={SourceType.Bilibili}>
                    Bilibili 开放平台
                  </SelectItem>
                  <SelectItem value={SourceType.Weibo}>
                    微博开放平台关键词
                  </SelectItem>
                  <SelectItem value={SourceType.GoogleAgentSearch}>
                    Google Agent Search（限定域）
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
            {form.sourceType === SourceType.HackerNews && (
              <div className="sm:col-span-2">
                <Label htmlFor="source-hacker-news-mode">榜单模式</Label>
                <Select
                  value={form.hackerNewsMode}
                  onValueChange={(value) =>
                    updateForm({ hackerNewsMode: value as HackerNewsMode })
                  }
                >
                  <SelectTrigger id="source-hacker-news-mode" className="mt-2">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="top">热门榜单</SelectItem>
                    <SelectItem value="best">最佳榜单</SelectItem>
                    <SelectItem value="new">最新项目</SelectItem>
                  </SelectContent>
                </Select>
                <p className="mt-1 text-xs text-muted-foreground">
                  热门和最佳模式会在每轮重复观测官方 score
                  与评论数，为事件热度和趋势提供连续指标。
                </p>
              </div>
            )}
            <div>
              <Label htmlFor="source-auth-type">授权方式</Label>
              <Select
                value={form.authType}
                disabled={
                  form.sourceType === SourceType.X ||
                  form.sourceType === SourceType.BingGrounding ||
                  form.sourceType === SourceType.Bilibili ||
                  form.sourceType === SourceType.Weibo ||
                  form.sourceType === SourceType.GoogleAgentSearch
                }
                onValueChange={(value) =>
                  updateForm({
                    authType: value as AuthType,
                    credential: value === "none" ? "" : form.credential,
                  })
                }
              >
                <SelectTrigger id="source-auth-type" className="mt-2">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">无凭据</SelectItem>
                  <SelectItem value="api_key">API Key</SelectItem>
                  <SelectItem value="bearer">Bearer Token</SelectItem>
                  <SelectItem value="oauth2">OAuth 2.0</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="sm:col-span-2">
              <Label htmlFor="source-endpoint">接口地址</Label>
              <Input
                id="source-endpoint"
                className="mt-2"
                value={form.endpoint}
                onChange={(event) =>
                  updateForm({ endpoint: event.target.value })
                }
                placeholder={
                  form.sourceType === SourceType.BingGrounding
                    ? "https://account.services.ai.azure.com/api/projects/project/toolboxes/web-search/versions/1/mcp?api-version=v1"
                    : "https://example.com/feed.xml"
                }
                readOnly={
                  form.sourceType === SourceType.HackerNews ||
                  form.sourceType === SourceType.X ||
                  form.sourceType === SourceType.Bilibili ||
                  form.sourceType === SourceType.Weibo ||
                  form.sourceType === SourceType.GoogleAgentSearch
                }
              />
              {form.sourceType === SourceType.X && (
                <p className="mt-1 text-xs text-muted-foreground">
                  固定使用官方 Recent Search；创建后先完成健康探测，再手动启用。
                </p>
              )}
              {form.sourceType === SourceType.BingGrounding && (
                <p className="mt-1 text-xs text-muted-foreground">
                  填写版本固定的 Foundry Toolbox MCP
                  地址；创建后先探测工具契约，再手动启用。
                </p>
              )}
              {form.sourceType === SourceType.Bilibili && (
                <p className="mt-1 text-xs text-muted-foreground">
                  固定使用官方开放平台；创建后先校验授权账号和
                  scopes，再手动启用。
                </p>
              )}
              {form.sourceType === SourceType.Weibo && (
                <p className="mt-1 text-xs text-muted-foreground">
                  固定使用微博官方 CLI
                  API；创建后先验证账号和关键词命令可用性，再手动启用。
                </p>
              )}
              {form.sourceType === SourceType.GoogleAgentSearch && (
                <p className="mt-1 text-xs text-muted-foreground">
                  固定使用 Discovery Engine v1；创建后先验证 ServingConfig
                  搜索权限，再手动启用。
                </p>
              )}
            </div>
            {form.sourceType === SourceType.Bilibili && (
              <div className="sm:col-span-2">
                <Label htmlFor="source-bilibili-open-id">授权账号 OpenID</Label>
                <Input
                  id="source-bilibili-open-id"
                  className="mono mt-2"
                  value={form.bilibiliOpenID}
                  onChange={(event) =>
                    updateForm({ bilibiliOpenID: event.target.value })
                  }
                  placeholder="从开放平台授权结果复制 OpenID"
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  仅支持已授权账号；公共 UID、@账号与主页地址不会被解析或抓取。
                </p>
              </div>
            )}
            {form.sourceType === SourceType.GoogleAgentSearch && (
              <>
                <div>
                  <Label htmlFor="source-google-location">数据位置</Label>
                  <Select
                    value={form.googleLocation}
                    onValueChange={(value) => {
                      const location = value as GoogleLocation;
                      updateForm({
                        googleLocation: location,
                        googleServingConfig: "",
                        endpoint: GOOGLE_AGENT_SEARCH_ENDPOINTS[location],
                      });
                    }}
                  >
                    <SelectTrigger id="source-google-location" className="mt-2">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="global">Global</SelectItem>
                      <SelectItem value="us">United States</SelectItem>
                      <SelectItem value="eu">European Union</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div>
                  <Label htmlFor="source-google-serving-config">
                    ServingConfig 资源名
                  </Label>
                  <Input
                    id="source-google-serving-config"
                    className="mono mt-2"
                    value={form.googleServingConfig}
                    onChange={(event) =>
                      updateForm({ googleServingConfig: event.target.value })
                    }
                    placeholder={`projects/project-id/locations/${form.googleLocation}/collections/default_collection/dataStores/data-store/servingConfigs/default_config`}
                    aria-invalid={
                      form.googleServingConfig.length > 0 &&
                      (!googleServingConfigPattern.test(
                        form.googleServingConfig.trim()
                      ) ||
                        !form.googleServingConfig.includes(
                          `/locations/${form.googleLocation}/`
                        ))
                    }
                  />
                </div>
              </>
            )}
            {form.authType !== "none" && (
              <div className="sm:col-span-2">
                <Label htmlFor="source-credential">访问凭据</Label>
                <PasswordInput
                  id="source-credential"
                  className="mono mt-2"
                  value={form.credential}
                  onChange={(event) =>
                    updateForm({ credential: event.target.value })
                  }
                  placeholder={
                    form.sourceType === SourceType.Bilibili
                      ? '{"client_id":"…","app_secret":"…","access_token":"…"}'
                      : "粘贴 API Key 或 Token"
                  }
                  autoComplete="new-password"
                  maxLength={16 * 1024}
                  aria-invalid={!credentialValid}
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  仅在本次提交中使用；服务端认证加密后保存，任何页面、日志和 API 响应都不会回显明文。
                </p>
              </div>
            )}
            <div className="sm:col-span-2">
              <Label htmlFor="source-terms-url">条款与政策地址</Label>
              <Input
                id="source-terms-url"
                className="mt-2"
                value={form.termsPolicyURL}
                onChange={(event) =>
                  updateForm({ termsPolicyURL: event.target.value })
                }
                placeholder="https://example.com/terms"
                aria-invalid={!validHTTPSURL(form.termsPolicyURL)}
                readOnly={
                  form.sourceType === SourceType.Bilibili ||
                  form.sourceType === SourceType.Weibo ||
                  form.sourceType === SourceType.GoogleAgentSearch
                }
              />
            </div>
            <div className="sm:col-span-2 rounded-md border border-border p-4">
              <p className="text-sm font-medium">合规与保留</p>
              <div className="mt-3 grid gap-3 sm:grid-cols-3">
                {complianceOptions.map((option) => (
                  <label
                    key={option.key}
                    className="flex cursor-pointer items-center gap-2 text-sm"
                  >
                    <Checkbox
                      aria-label={option.label}
                      checked={form[option.key]}
                      onCheckedChange={(checked) =>
                        updateForm({ [option.key]: checked === true })
                      }
                      disabled={
                        (form.sourceType === SourceType.BingGrounding &&
                          (option.key === "allowBodyStorage" ||
                            option.key === "requiresAttribution")) ||
                        (form.sourceType === SourceType.Bilibili &&
                          (option.key === "allowBodyStorage" ||
                            option.key === "requiresAttribution" ||
                            option.key === "requiresDeletionSync")) ||
                        (form.sourceType === SourceType.Weibo &&
                          (option.key === "allowBodyStorage" ||
                            option.key === "requiresAttribution" ||
                            option.key === "requiresDeletionSync")) ||
                        (form.sourceType === SourceType.GoogleAgentSearch &&
                          (option.key === "allowBodyStorage" ||
                            option.key === "requiresAttribution"))
                      }
                    />
                    {option.label}
                  </label>
                ))}
              </div>
              <p className="mt-3 text-xs text-muted-foreground">
                {form.sourceType === SourceType.BingGrounding
                  ? "只保存 Foundry 模型生成的派生摘要和必要引用，不把它标记为原始网页正文或来源指标。"
                  : form.sourceType === SourceType.Bilibili
                  ? "仅轮询已授权账号的视频和专栏；授权撤销后自动停用，并按来源政策同步删除状态。"
                  : form.sourceType === SourceType.Weibo
                  ? "只调用当前 Token 已获套餐授权的关键词命令；不可见或删除提示不保存正文，也不抓取微博网页。"
                  : form.sourceType === SourceType.GoogleAgentSearch
                  ? "只保存已配置数据存储返回的标题、HTTPS 链接和 snippet；不抓取结果网页，也不把限定域搜索描述为全网搜索。"
                  : "只保存来源 Feed 实际提供的正文/摘要，不抓取原网页；启用前确认来源条款。"}
              </p>
            </div>
            {form.sourceType === SourceType.BingGrounding && (
              <Alert className="sm:col-span-2">
                <TriangleAlert />
                <div className="mb-1 font-medium leading-none tracking-tight">
                  Grounding 数据边界确认
                </div>
                <AlertDescription>
                  <label className="mt-2 flex cursor-pointer items-start gap-2 text-sm text-foreground">
                    <Checkbox
                      aria-label="确认 Grounding 数据边界与额外条款"
                      checked={form.groundingDataBoundaryApproved}
                      onCheckedChange={(checked) =>
                        updateForm({
                          groundingDataBoundaryApproved: checked === true,
                        })
                      }
                    />
                    <span>
                      我已确认 Microsoft DPA
                      不适用于该能力，数据可能离开既定合规与地理边界，并接受额外条款及费用。
                    </span>
                  </label>
                  <p className="mt-3 text-xs text-muted-foreground">
                    Web Search
                    返回模型生成的派生摘要和引用，不是原始搜索结果或网页正文；查询不得包含身份、凭据或其他敏感数据。
                  </p>
                </AlertDescription>
              </Alert>
            )}
            {form.sourceType === SourceType.Weibo && (
              <Alert className="sm:col-span-2">
                <TriangleAlert />
                <div className="mb-1 font-medium leading-none tracking-tight">
                  微博官方能力前提
                </div>
                <AlertDescription>
                  账号须完成开发者认证、开通套餐或试用并取得 API
                  Token；健康探测还会确认 search statuses/limited 当前可用。MVP
                  不支持账号时间线、热搜页或网页抓取。
                </AlertDescription>
              </Alert>
            )}
            {form.sourceType === SourceType.GoogleAgentSearch && (
              <Alert className="sm:col-span-2">
                <TriangleAlert />
                <div className="mb-1 font-medium leading-none tracking-tight">
                  Google 搜索迁移边界
                </div>
                <AlertDescription>
                  Custom Search JSON API 已关闭新客户，并将在 2027-01-01
                  停用存量服务。此来源仅搜索你在 Agent Search
                  中配置的限定域数据存储；全网搜索需要另行取得 Google
                  正式方案，系统不会降级抓取 Google 搜索页。
                </AlertDescription>
              </Alert>
            )}
            <div>
              <Label htmlFor="source-languages">允许语言</Label>
              <Input
                id="source-languages"
                className="mt-2"
                value={form.allowedLanguages}
                onChange={(event) =>
                  updateForm({ allowedLanguages: event.target.value })
                }
                placeholder="zh-CN, en"
              />
            </div>
            <div>
              <Label htmlFor="source-regions">允许地区</Label>
              <Input
                id="source-regions"
                className="mt-2"
                value={form.allowedRegions}
                onChange={(event) =>
                  updateForm({ allowedRegions: event.target.value })
                }
                placeholder="CN, US"
              />
            </div>
            {numericOptions.map((option) => (
              <div key={option.key}>
                <Label htmlFor={`source-${option.key}`}>{option.label}</Label>
                <Input
                  id={`source-${option.key}`}
                  className="mono mt-2"
                  type="number"
                  min={option.min}
                  max={option.max}
                  value={form[option.key]}
                  onChange={(event) =>
                    updateForm({ [option.key]: Number(event.target.value) })
                  }
                  disabled={
                    form.sourceType === SourceType.BingGrounding &&
                    option.key === "maxPagesPerRun"
                  }
                />
              </div>
            ))}
          </div>
          <DialogFooter className="border-t border-border px-6 py-4">
            <Button
              type="button"
              variant="outline"
              onClick={() => changeOpen(false)}
            >
              取消
            </Button>
            <Button type="submit" disabled={busy || !valid}>
              {busy && <Loader2 className="animate-spin" />}
              创建连接
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
