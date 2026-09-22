"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { RotateCcwIcon } from "lucide-react";

import { listSourceCapabilities } from "@/api/laiyuannengli";
import { PageState } from "@/components/system/page-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ApiRequestError } from "@/request";

type MatrixState =
  | { status: "loading" }
  | { status: "ready"; platforms: HotKeyAPI.SourcePlatformView[] }
  | { status: "error"; message: string; requestId?: string };

type MatrixResult =
  Exclude<MatrixState, { status: "loading" }> | { status: "unauthenticated" };

const STATUS_LABELS: Record<HotKeyAPI.SourceCapabilityStatus, string> = {
  unconfigured: "未配置",
  pending_verification: "待验证",
  available: "可用",
  authentication_required: "需重新授权",
  restricted: "受限",
  disabled: "已停用",
};

const PLATFORM_STATUS_LABELS: Record<HotKeyAPI.SourcePlatformStatus, string> = {
  ...STATUS_LABELS,
  partial: "部分可用",
};

const STATUS_VARIANTS: Record<
  HotKeyAPI.SourceCapabilityStatus | HotKeyAPI.SourcePlatformStatus,
  "default" | "secondary" | "destructive" | "outline"
> = {
  unconfigured: "outline",
  pending_verification: "secondary",
  available: "default",
  authentication_required: "outline",
  restricted: "outline",
  disabled: "outline",
  partial: "secondary",
};

function isInvalidSession(error: unknown): boolean {
  return error instanceof ApiRequestError && error.code === "invalid_session";
}

async function readCapabilities(): Promise<MatrixResult> {
  try {
    const page = await listSourceCapabilities();
    return { status: "ready", platforms: page.items };
  } catch (error) {
    if (isInvalidSession(error)) {
      return { status: "unauthenticated" };
    }
    return {
      status: "error",
      message:
        error instanceof ApiRequestError
          ? error.message
          : "来源能力加载失败，请稍后重试。",
      requestId: error instanceof ApiRequestError ? error.requestId : undefined,
    };
  }
}

function formatTime(value: string | null): string {
  if (value === null) {
    return "尚无记录";
  }
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function CapabilityStatus({
  value,
}: {
  value: HotKeyAPI.SourceEntryPointView;
}) {
  return (
    <div className="min-w-0 space-y-2">
      <Badge variant={STATUS_VARIANTS[value.status]}>
        {STATUS_LABELS[value.status]}
      </Badge>
      <p className="text-muted-foreground text-xs leading-5 whitespace-normal">
        {value.next_action}
      </p>
      <p className="text-muted-foreground text-xs whitespace-normal">
        最近检查：{formatTime(value.last_checked_at)}
      </p>
      {value.last_persisted_success_at ? (
        <p className="text-muted-foreground text-xs whitespace-normal">
          最近持久成功：{formatTime(value.last_persisted_success_at)}
        </p>
      ) : null}
    </div>
  );
}

function PlatformSection({
  platform,
}: {
  platform: HotKeyAPI.SourcePlatformView;
}) {
  return (
    <section
      aria-labelledby={`source-${platform.source_key}`}
      className="mt-10"
    >
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h2
              id={`source-${platform.source_key}`}
              className="text-xl font-medium"
            >
              {platform.display_name}
            </h2>
            <Badge variant="outline">
              {platform.rollout_role === "required" ? "首版必需" : "候选平台"}
            </Badge>
            <Badge variant={STATUS_VARIANTS[platform.status]}>
              {PLATFORM_STATUS_LABELS[platform.status]}
            </Badge>
          </div>
          <p className="text-muted-foreground mt-2 text-sm">
            {platform.connection_version === null
              ? "尚未配置连接"
              : `当前连接版本 v${platform.connection_version}`}
          </p>
        </div>
      </div>

      <div className="mt-5 hidden md:block">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-40">能力</TableHead>
              <TableHead>手动入口</TableHead>
              <TableHead>定时入口</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {platform.capabilities.map((capability) => (
              <TableRow key={capability.capability}>
                <TableCell className="font-medium">
                  {capability.display_name}
                </TableCell>
                <TableCell className="max-w-sm align-top whitespace-normal">
                  <CapabilityStatus value={capability.manual} />
                </TableCell>
                <TableCell className="max-w-sm align-top whitespace-normal">
                  <CapabilityStatus value={capability.scheduled} />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <div className="mt-5 grid gap-3 md:hidden">
        {platform.capabilities.map((capability) => (
          <article
            key={capability.capability}
            className="bg-muted rounded-2xl p-5"
          >
            <h3 className="font-medium">{capability.display_name}</h3>
            <div className="mt-4 grid gap-5">
              <div>
                <p className="text-muted-foreground mb-2 text-xs font-medium tracking-wider uppercase">
                  手动入口
                </p>
                <CapabilityStatus value={capability.manual} />
              </div>
              <div>
                <p className="text-muted-foreground mb-2 text-xs font-medium tracking-wider uppercase">
                  定时入口
                </p>
                <CapabilityStatus value={capability.scheduled} />
              </div>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}

function LoadingMatrix() {
  return (
    <div aria-label="正在读取来源能力" className="mt-10 space-y-8">
      {[0, 1].map((item) => (
        <div key={item} className="space-y-4">
          <Skeleton className="h-7 w-48" />
          <Skeleton className="h-40 w-full rounded-2xl" />
        </div>
      ))}
    </div>
  );
}

export function SourceCapabilityMatrix() {
  const router = useRouter();
  const [state, setState] = useState<MatrixState>({ status: "loading" });

  const reload = useCallback(async () => {
    setState({ status: "loading" });
    const result = await readCapabilities();
    if (result.status === "unauthenticated") {
      router.replace("/login");
      return;
    }
    setState(result);
  }, [router]);

  useEffect(() => {
    let isCurrent = true;
    void readCapabilities().then((result) => {
      if (!isCurrent) {
        return;
      }
      if (result.status === "unauthenticated") {
        router.replace("/login");
        return;
      }
      setState(result);
    });
    return () => {
      isCurrent = false;
    };
  }, [router]);

  if (state.status === "error") {
    return (
      <PageState
        eyebrow="来源能力"
        title="暂时无法读取来源状态"
        description={
          state.requestId
            ? `${state.message} 请求编号：${state.requestId}`
            : state.message
        }
        action={
          <Button onClick={() => void reload()}>
            <RotateCcwIcon data-icon="inline-start" />
            重新加载
          </Button>
        }
      />
    );
  }

  return (
    <div className="bg-background min-h-screen">
      <header className="mx-auto flex h-16 max-w-7xl items-center justify-between px-5 sm:px-8 xl:px-0">
        <Link href="/events" className="flex items-center gap-2.5 font-medium">
          <span className="bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-lg font-mono text-xs">
            HK
          </span>
          <span>HotKey</span>
        </Link>
        <Button asChild variant="ghost" size="navigation">
          <Link href="/events">返回事件</Link>
        </Button>
      </header>

      <main className="mx-auto max-w-7xl px-5 py-12 sm:px-8 sm:py-16 xl:px-0">
        <p className="text-muted-foreground font-mono text-xs tracking-wider uppercase">
          Sources
        </p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight sm:text-4xl">
          来源能力
        </h1>
        <p className="text-muted-foreground mt-4 max-w-2xl leading-7">
          按平台、能力和入口查看已经持久验证的状态。官方声明、探测成功或旧连接记录都不会自动标记为可用。
        </p>

        {state.status === "loading" ? <LoadingMatrix /> : null}
        {state.status === "ready" && state.platforms.length === 0 ? (
          <section className="bg-muted mt-10 rounded-2xl px-6 py-12 text-center">
            <h2 className="font-medium">暂无来源目录</h2>
            <p className="text-muted-foreground mt-2 text-sm">
              目录尚未加载，不代表平台返回空结果。
            </p>
          </section>
        ) : null}
        {state.status === "ready"
          ? state.platforms.map((platform) => (
              <PlatformSection key={platform.source_key} platform={platform} />
            ))
          : null}
      </main>
    </div>
  );
}
