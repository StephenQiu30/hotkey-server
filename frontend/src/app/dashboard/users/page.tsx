"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  AlertCircle,
  Loader2,
  RotateCcw,
  Trash2,
  Users,
} from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PageShell } from "@/layouts/PageShell";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";
import { CursorPagination } from "@/components/dashboard/CursorPagination";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";
import {
  deleteUsersId,
  getUsers,
  patchUsersId,
  postUsersIdRestore,
} from "@/services/hotkey/hotkey-server/identity";

const PAGE_SIZE = 10;

const roleLabels: Record<string, string> = {
  admin: "管理员",
  analyst: "分析者",
  editor: "编辑者",
  viewer: "查看者",
};

export default function UsersPage() {
  const actor = useAuthStore((state) => state.user);
  const canManage = actor?.role === UserRole.Admin;
  const [users, setUsers] = useState<HotKeyAPI.UserResponse[]>([]);
  const [loading, setLoading] = useState(canManage);
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [role, setRole] = useState("all");
  const [status, setStatus] = useState("all");
  const [page, setPage] = useState(1);
  const [actionId, setActionId] = useState<number>();
  const [deleteTarget, setDeleteTarget] = useState<HotKeyAPI.UserResponse>();

  const load = useCallback(async () => {
    if (!canManage) return;
    setLoading(true);
    setError("");
    try {
      const result = await getUsers();
      setUsers(result.data ?? []);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "用户列表加载失败");
    } finally {
      setLoading(false);
    }
  }, [canManage]);

  useEffect(() => {
    void load();
  }, [load]);

  const filteredUsers = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase();
    return users.filter((user) => {
      const deleted = Boolean(user.deleted_at);
      const matchesQuery =
        !normalized ||
        user.email?.toLocaleLowerCase().includes(normalized) ||
        user.display_name?.toLocaleLowerCase().includes(normalized);
      const matchesRole = role === "all" || user.role === role;
      const matchesStatus =
        status === "all" ||
        (status === "deleted" ? deleted : !deleted && user.status === status);
      return matchesQuery && matchesRole && matchesStatus;
    });
  }, [query, role, status, users]);

  useEffect(() => setPage(1), [query, role, status]);

  const visibleUsers = filteredUsers.slice(
    (page - 1) * PAGE_SIZE,
    page * PAGE_SIZE
  );

  const replaceUser = (updated?: HotKeyAPI.UserResponse) => {
    if (updated?.id == null) return;
    setUsers((current) =>
      current.map((user) => (user.id === updated.id ? updated : user))
    );
  };

  const updateUser = async (
    target: HotKeyAPI.UserResponse,
    update: HotKeyAPI.UpdateUserRequest
  ) => {
    if (target.id == null) return;
    setActionId(target.id);
    try {
      const result = await patchUsersId({ id: target.id }, update);
      replaceUser(result.data);
      toast.success("用户权限已更新");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "用户更新失败");
    } finally {
      setActionId(undefined);
    }
  };

  const deleteUser = async () => {
    if (deleteTarget?.id == null) return;
    setActionId(deleteTarget.id);
    try {
      const result = await deleteUsersId({ id: deleteTarget.id });
      replaceUser(result.data);
      setDeleteTarget(undefined);
      toast.success("用户已软删除，会话已撤销");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "用户删除失败");
    } finally {
      setActionId(undefined);
    }
  };

  const restoreUser = async (target: HotKeyAPI.UserResponse) => {
    if (target.id == null) return;
    setActionId(target.id);
    try {
      const result = await postUsersIdRestore({ id: target.id });
      replaceUser(result.data);
      toast.success("用户已恢复为禁用状态");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "用户恢复失败");
    } finally {
      setActionId(undefined);
    }
  };

  if (!canManage) {
    return null;
  }

  return (
    <PageShell>
      <PageHeader
        eyebrow="Administration"
        title="用户与权限"
        description="调整固定角色、禁用账户，并以可恢复软删除管理成员。"
      />

      <Card className="mt-6 overflow-hidden">
        <CardContent className="grid gap-3 border-b p-4 md:grid-cols-[minmax(0,1fr)_180px_180px]">
          <Input
            type="search"
            aria-label="搜索用户"
            placeholder="搜索邮箱或显示名称"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <Select value={role} onValueChange={setRole}>
            <SelectTrigger aria-label="筛选角色">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部角色</SelectItem>
              <SelectItem value="admin">管理员</SelectItem>
              <SelectItem value="analyst">分析者</SelectItem>
              <SelectItem value="editor">编辑者</SelectItem>
              <SelectItem value="viewer">查看者</SelectItem>
            </SelectContent>
          </Select>
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger aria-label="筛选状态">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部状态</SelectItem>
              <SelectItem value="active">启用</SelectItem>
              <SelectItem value="disabled">禁用</SelectItem>
              <SelectItem value="deleted">已删除</SelectItem>
            </SelectContent>
          </Select>
        </CardContent>

        {error ? (
          <div className="p-4">
            <Alert variant="destructive">
              <AlertCircle />
              <AlertTitle>无法加载用户</AlertTitle>
              <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
                <span>{error}</span>
                <Button size="sm" variant="outline" onClick={() => void load()}>
                  重试
                </Button>
              </AlertDescription>
            </Alert>
          </div>
        ) : loading ? (
          <div className="space-y-3 p-5" aria-label="正在加载用户">
            {Array.from({ length: 4 }, (_, index) => (
              <Skeleton key={index} className="h-12 w-full" />
            ))}
          </div>
        ) : visibleUsers.length === 0 ? (
          <Empty className="min-h-64 rounded-none border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Users />
              </EmptyMedia>
              <EmptyTitle>没有匹配用户</EmptyTitle>
              <EmptyDescription>调整搜索词或筛选条件后重试。</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <Table aria-label="用户列表" className="min-w-[880px]" scrollAreaLabel="用户列表">
            <TableHeader>
              <TableRow>
                <TableHead>用户</TableHead>
                <TableHead>角色</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>最近更新</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {visibleUsers.map((target) => {
                const deleted = Boolean(target.deleted_at);
                const busy = actionId === target.id;
                return (
                  <TableRow key={target.id}>
                    <TableCell>
                      <div className="flex items-center font-medium">
                        {target.display_name || "未命名用户"}
                        {target.id === actor?.id ? (
                          <Badge variant="outline" className="ml-2">
                            当前账户
                          </Badge>
                        ) : null}
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {target.email}
                      </p>
                    </TableCell>
                    <TableCell>
                      <Select
                        disabled={deleted || busy}
                        value={target.role}
                        onValueChange={(value) =>
                          void updateUser(target, { role: value as UserRole })
                        }
                      >
                        <SelectTrigger
                          aria-label={`设置 ${target.email} 的角色`}
                          className="w-28"
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {Object.entries(roleLabels).map(([value, label]) => (
                            <SelectItem key={value} value={value}>
                              {label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          deleted
                            ? "destructive"
                            : target.status === "active"
                            ? "default"
                            : "secondary"
                        }
                      >
                        {deleted
                          ? "已删除"
                          : target.status === "active"
                          ? "启用"
                          : "禁用"}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {target.updated_at
                        ? new Date(target.updated_at).toLocaleString("zh-CN")
                        : "—"}
                    </TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-2">
                        {deleted ? (
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={busy}
                            aria-label={`恢复 ${target.email}`}
                            onClick={() => void restoreUser(target)}
                          >
                            {busy ? (
                              <Loader2 className="animate-spin" />
                            ) : (
                              <RotateCcw />
                            )}
                            恢复
                          </Button>
                        ) : (
                          <>
                            <Button
                              size="sm"
                              variant="outline"
                              disabled={busy}
                              aria-label={`${
                                target.status === "active" ? "禁用" : "启用"
                              } ${target.email}`}
                              onClick={() =>
                                void updateUser(target, {
                                  status:
                                    target.status === "active"
                                      ? "disabled"
                                      : "active",
                                })
                              }
                            >
                              {busy ? (
                                <Loader2 className="animate-spin" />
                              ) : null}
                              {target.status === "active" ? "禁用" : "启用"}
                            </Button>
                            <Button
                              size="icon"
                              className="h-8 w-8"
                              variant="ghost"
                              disabled={busy}
                              aria-label={`删除 ${target.email}`}
                              onClick={() => setDeleteTarget(target)}
                            >
                              <Trash2 />
                            </Button>
                          </>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        )}

        {!loading && !error ? (
          <CursorPagination
            page={page}
            pageSize={PAGE_SIZE}
            hasNext={page * PAGE_SIZE < filteredUsers.length}
            onPrevious={() => setPage((current) => Math.max(1, current - 1))}
            onNext={() => setPage((current) => current + 1)}
          />
        ) : null}
      </Card>

      <p className="mt-3 text-xs text-muted-foreground">
        角色、状态、删除与恢复操作均由服务端记录操作者、变更前后值、请求 ID
        和结果。
      </p>

      <ConfirmDeleteDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => !open && setDeleteTarget(undefined)}
        title="删除用户？"
        description="账户将被软删除，所有活动会话立即撤销；管理员可稍后恢复为禁用状态。"
        resourceName={deleteTarget?.email ?? "用户"}
        onConfirm={deleteUser}
        loading={deleteTarget?.id === actionId}
      />
    </PageShell>
  );
}
