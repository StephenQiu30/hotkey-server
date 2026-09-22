"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";

import { updateSourceConnection } from "@/api/laiyuannengli";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { ApiRequestError } from "@/request";

export function SourceConnectionActions({
  platform,
  onChanged,
}: {
  platform: HotKeyAPI.SourcePlatformView;
  onChanged: () => Promise<void>;
}) {
  const router = useRouter();
  const submitting = useRef(false);
  const [pending, setPending] = useState(false);
  const [confirm, setConfirm] =
    useState<HotKeyAPI.SourceConnectionStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const active = platform.connection_status === "active";
  const label =
    platform.connection_version === null
      ? "配置连接"
      : platform.credential_update_available
        ? "替换连接"
        : active
          ? "已配置"
          : "重新启用";

  async function save(status: HotKeyAPI.SourceConnectionStatus) {
    if (submitting.current) return;
    submitting.current = true;
    setPending(true);
    setError(null);
    setNotice(null);
    try {
      await updateSourceConnection(
        { source_key: platform.source_key },
        {
          expected_version: platform.connection_version ?? 0,
          status,
        },
      );
      setConfirm(null);
      setNotice(
        status === "disabled"
          ? "连接已停用，历史资料仍可读取。"
          : "连接已保存，能力仍需重新验证。",
      );
      await onChanged();
    } catch (failure) {
      if (
        failure instanceof ApiRequestError &&
        failure.code === "invalid_session"
      ) {
        router.replace("/login");
        return;
      }
      setConfirm(null);
      setError(
        failure instanceof ApiRequestError
          ? `${failure.message}${failure.requestId ? ` 请求编号：${failure.requestId}` : ""}`
          : "连接更新失败，请刷新状态后重试。",
      );
    } finally {
      submitting.current = false;
      setPending(false);
    }
  }

  return (
    <div
      role="group"
      className="flex max-w-md flex-col gap-3"
      aria-label={`${platform.display_name}连接管理`}
    >
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={
            pending ||
            !platform.credential_configured ||
            (active && !platform.credential_update_available)
          }
          onClick={() => setConfirm("active")}
        >
          {pending ? "正在保存…" : label}
        </Button>
        {active ? (
          <Button
            variant="ghost"
            disabled={pending}
            onClick={() => setConfirm("disabled")}
          >
            停用连接
          </Button>
        ) : null}
      </div>
      {!platform.credential_configured ? (
        <p className="text-muted-foreground text-sm">
          请维护者先配置服务端凭据。页面不接收或显示会话秘密。
        </p>
      ) : null}
      {error ? (
        <div className="flex flex-col items-start gap-2">
          <p role="alert" className="text-destructive text-sm">
            {error}
          </p>
          <Button
            variant="outline"
            disabled={pending}
            onClick={() => void onChanged()}
          >
            刷新状态
          </Button>
        </div>
      ) : null}
      {notice ? (
        <p role="status" className="text-muted-foreground text-sm">
          {notice}
        </p>
      ) : null}
      <AlertDialog
        open={confirm !== null}
        onOpenChange={(open) => {
          if (!open && !pending) setConfirm(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirm === "disabled"
                ? "停用"
                : platform.connection_version === null
                  ? "配置"
                  : platform.credential_update_available
                    ? "替换"
                    : "重新启用"}
              {platform.display_name}连接？
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirm === "disabled"
                ? "停止接受该连接的新任务。历史资料保留，不删除已有结果。"
                : "使用维护者已配置的服务端凭据。新版本需要重新验证，不沿用旧版本成功状态，也不会自动发起平台请求。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={pending}>取消</AlertDialogCancel>
            <Button
              disabled={pending}
              onClick={() => {
                if (confirm) void save(confirm);
              }}
            >
              {pending ? "正在保存…" : "确认"}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
