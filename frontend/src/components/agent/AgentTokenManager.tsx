"use client";

import { useCallback, useEffect, useState } from "react";
import { AlertCircle, Check, Copy, KeyRound, Loader2, Plus } from "lucide-react";
import { useAuthStore } from "@/stores/authStore";
import {
  getAgentTokens,
  postAgentTokens,
  postAgentTokensIdRevoke,
} from "@/services/hotkey/hotkey-server/agentAccess";
import { UserRole } from "@/lib/domainEnums";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
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
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

type AgentScope = HotKeyAPI.CreateTokenRequest["scopes"][number];

const scopeOptions: Array<{
  value: AgentScope;
  label: string;
  description: string;
  elevated?: boolean;
}> = [
  { value: "monitors.read", label: "监控读取", description: "读取监控及其配置" },
  { value: "events.read", label: "事件读取", description: "读取事件、热度和情报" },
  { value: "contents.read", label: "内容读取", description: "读取内容及正文文档" },
  { value: "reports.read", label: "报告读取", description: "读取日报与周报" },
  { value: "alerts.write", label: "告警处理", description: "读取、确认和解决告警" },
  { value: "search.run", label: "执行采集", description: "手动触发监控采集", elevated: true },
];

const scopeLabels = Object.fromEntries(
  scopeOptions.map((scope) => [scope.value, scope.label]),
) as Record<AgentScope, string>;

function formatDate(value?: string): string {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString("zh-CN", { hour12: false });
}

function tokenStatus(token: HotKeyAPI.TokenResponse): {
  label: string;
  active: boolean;
  variant: "default" | "secondary" | "outline";
} {
  if (token.revoked_at) return { label: "已撤销", active: false, variant: "outline" };
  if (!token.expires_at || new Date(token.expires_at).getTime() <= Date.now()) {
    return { label: "已过期", active: false, variant: "secondary" };
  }
  return { label: "有效", active: true, variant: "default" };
}

export function AgentTokenManager() {
  const canRunSearch = useAuthStore(
    (state) => state.user?.role === UserRole.Editor || state.user?.role === UserRole.Admin,
  );
  const availableScopes = scopeOptions.filter((scope) => !scope.elevated || canRunSearch);
  const [tokens, setTokens] = useState<HotKeyAPI.TokenResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string>();
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [lifetimeDays, setLifetimeDays] = useState("30");
  const [scopes, setScopes] = useState<AgentScope[]>([]);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string>();
  const [created, setCreated] = useState<HotKeyAPI.CreatedTokenResponse>();
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState<string>();
  const [revokeTarget, setRevokeTarget] = useState<HotKeyAPI.TokenResponse>();
  const [revoking, setRevoking] = useState(false);
  const [revokeError, setRevokeError] = useState<string>();

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError(undefined);
    try {
      const response = await getAgentTokens();
      setTokens(response.data ?? []);
    } catch (reason) {
      setLoadError(reason instanceof Error ? reason.message : "Agent Token 加载失败");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  function resetCreateForm() {
    setName("");
    setLifetimeDays("30");
    setScopes([]);
    setCreateError(undefined);
  }

  function toggleScope(scope: AgentScope, checked: boolean) {
    setScopes((current) =>
      checked ? [...current, scope] : current.filter((item) => item !== scope),
    );
  }

  async function createToken(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreateError(undefined);
    if (!name.trim() || scopes.length === 0) {
      setCreateError("请填写名称并至少选择一个权限范围");
      return;
    }
    setCreating(true);
    try {
      const response = await postAgentTokens({
        name: name.trim(),
        scopes,
        lifetime_days: Number(lifetimeDays),
      });
      if (!response.data?.token) throw new Error("服务未返回一次性凭据");
      setCreated(response.data);
      setCreateOpen(false);
      resetCreateForm();
      await load();
    } catch (reason) {
      setCreateError(reason instanceof Error ? reason.message : "Agent Token 创建失败");
    } finally {
      setCreating(false);
    }
  }

  async function copyCredential() {
    if (!created?.token) return;
    setCopyError(undefined);
    try {
      await navigator.clipboard.writeText(created.token);
      setCopied(true);
    } catch {
      setCopyError("复制失败，请手动复制凭据");
    }
  }

  function clearCreatedCredential() {
    setCreated(undefined);
    setCopied(false);
    setCopyError(undefined);
  }

  async function revokeToken() {
    if (!revokeTarget?.id || !revokeTarget.version) return;
    setRevoking(true);
    setRevokeError(undefined);
    try {
      const response = await postAgentTokensIdRevoke(
        { id: revokeTarget.id },
        { expected_version: revokeTarget.version },
      );
      const revoked = response.data;
      if (!revoked) throw new Error("服务未返回撤销结果");
      setTokens((current) =>
        current.map((token) => (token.id === revoked.id ? revoked : token)),
      );
      setRevokeTarget(undefined);
    } catch (reason) {
      setRevokeError(reason instanceof Error ? reason.message : "Agent Token 撤销失败");
    } finally {
      setRevoking(false);
    }
  }

  return (
    <Card>
      <CardHeader className="gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1.5">
          <CardTitle className="flex items-center gap-2 text-base" role="heading" aria-level={2}>
            <KeyRound className="h-4 w-4" />
            Agent Token
          </CardTitle>
          <CardDescription>
            为 Agent Skill 和外部 API 创建最小权限凭据；每位用户最多保留 10 个有效 Token。
          </CardDescription>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="h-4 w-4" />
          创建 Token
        </Button>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="space-y-3" aria-label="正在加载 Agent Token">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : loadError ? (
          <Alert variant="destructive">
            <AlertCircle />
            <AlertTitle>无法加载 Agent Token</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
              <span>{loadError}</span>
              <Button size="sm" variant="outline" onClick={() => void load()}>
                重试
              </Button>
            </AlertDescription>
          </Alert>
        ) : tokens.length === 0 ? (
          <div className="rounded-lg border border-dashed p-8 text-center">
            <KeyRound className="mx-auto h-5 w-5 text-muted-foreground" />
            <p className="mt-3 text-sm font-medium">尚未创建 Agent Token</p>
            <p className="mt-1 text-xs text-muted-foreground">创建后即可通过 Bearer 认证访问独立的 Agent API。</p>
          </div>
        ) : (
          <Table scrollAreaLabel="Agent Token 列表">
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>权限范围</TableHead>
                <TableHead>最近使用</TableHead>
                <TableHead>到期时间</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((token) => {
                const status = tokenStatus(token);
                return (
                  <TableRow key={token.id ?? token.token_prefix}>
                    <TableCell>
                      <div className="font-medium">{token.name || "未命名"}</div>
                      <code className="text-xs text-muted-foreground">{token.token_prefix}…</code>
                    </TableCell>
                    <TableCell>
                      <div className="flex max-w-80 flex-wrap gap-1">
                        {(token.scopes ?? []).map((scope) => (
                          <Badge key={scope} variant="outline" className="font-normal">
                            {scopeLabels[scope as AgentScope] ?? scope}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                      {formatDate(token.last_used_at)}
                    </TableCell>
                    <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                      {formatDate(token.expires_at)}
                    </TableCell>
                    <TableCell><Badge variant={status.variant}>{status.label}</Badge></TableCell>
                    <TableCell className="text-right">
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={!status.active}
                        onClick={() => {
                          setRevokeError(undefined);
                          setRevokeTarget(token);
                        }}
                      >
                        撤销
                      </Button>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}
      </CardContent>

      <Dialog
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open && !creating) resetCreateForm();
        }}
      >
        <DialogContent>
          <form onSubmit={createToken} className="space-y-5">
            <DialogHeader>
              <DialogTitle>创建 Agent Token</DialogTitle>
              <DialogDescription>凭据只显示一次。选择完成任务所需的最少权限。</DialogDescription>
            </DialogHeader>
            <div className="space-y-2">
              <Label htmlFor="agent-token-name">名称</Label>
              <Input
                id="agent-token-name"
                value={name}
                maxLength={64}
                autoComplete="off"
                placeholder="例如：研究助手"
                onChange={(event) => setName(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="agent-token-lifetime">有效期</Label>
              <Select value={lifetimeDays} onValueChange={setLifetimeDays}>
                <SelectTrigger id="agent-token-lifetime"><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="7">7 天</SelectItem>
                  <SelectItem value="30">30 天</SelectItem>
                  <SelectItem value="90">90 天</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <fieldset className="space-y-3">
              <legend className="text-sm font-medium">权限范围</legend>
              <div className="grid gap-3 sm:grid-cols-2">
                {availableScopes.map((scope) => (
                  <Label key={scope.value} className="flex cursor-pointer items-start gap-3 rounded-md border p-3 font-normal">
                    <Checkbox
                      aria-label={scope.label}
                      checked={scopes.includes(scope.value)}
                      onCheckedChange={(checked) => toggleScope(scope.value, checked === true)}
                    />
                    <span>
                      <span className="block text-sm font-medium">{scope.label}</span>
                      <span className="mt-0.5 block text-xs text-muted-foreground">{scope.description}</span>
                    </span>
                  </Label>
                ))}
              </div>
            </fieldset>
            {createError && <p className="text-sm text-destructive" role="alert">{createError}</p>}
            <DialogFooter>
              <Button type="button" variant="outline" disabled={creating} onClick={() => setCreateOpen(false)}>取消</Button>
              <Button type="submit" disabled={creating}>
                {creating && <Loader2 className="h-4 w-4 animate-spin" />}
                创建
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(created)}
        onOpenChange={(open) => {
          if (!open) clearCreatedCredential();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>保存一次性凭据</DialogTitle>
            <DialogDescription>关闭后无法再次查看。请立即复制到受控的密钥存储中。</DialogDescription>
          </DialogHeader>
          <Alert>
            <KeyRound />
            <AlertTitle>Agent Token</AlertTitle>
            <AlertDescription>
              <code className="block break-all select-all rounded-md bg-muted p-3 text-xs text-foreground">{created?.token}</code>
            </AlertDescription>
          </Alert>
          <div className="min-h-5 text-xs" aria-live="polite">
            {copied ? <span className="inline-flex items-center gap-1"><Check className="h-3.5 w-3.5" />已复制</span> : copyError ? <span className="text-destructive">{copyError}</span> : null}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => void copyCredential()}>
              <Copy className="h-4 w-4" />复制凭据
            </Button>
            <Button onClick={clearCreatedCredential}>我已保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={Boolean(revokeTarget)} onOpenChange={(open) => !open && !revoking && setRevokeTarget(undefined)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>撤销“{revokeTarget?.name}”</AlertDialogTitle>
            <AlertDialogDescription>撤销后该凭据的下一次 API 请求会立即失败，此操作不可恢复。</AlertDialogDescription>
          </AlertDialogHeader>
          {revokeError && <p className="text-sm text-destructive" role="alert">{revokeError}</p>}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={revoking}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={revoking}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={(event) => {
                event.preventDefault();
                void revokeToken();
              }}
            >
              {revoking && <Loader2 className="h-4 w-4 animate-spin" />}
              确认撤销
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
