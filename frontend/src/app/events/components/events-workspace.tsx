"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import {
  FileTextIcon,
  ListChecksIcon,
  LogOutIcon,
  NewspaperIcon,
  PlusIcon,
  RadioIcon,
  RotateCcwIcon,
} from "lucide-react";

import { deleteIdentitySession, getIdentityWorkspace } from "@/api/identity";
import { BrandLockup } from "@/components/brand/brand-lockup";
import { TopicList } from "@/components/monitors/topic-list";
import { PageState } from "@/components/system/page-state";
import { Button } from "@/components/ui/button";
import { ApiRequestError } from "@/request";

type WorkspaceState =
  | { status: "loading" }
  | { status: "ready"; workspace: HotKeyAPI.IdentityWorkspaceView }
  | { status: "error"; message: string; requestId?: string };

type WorkspaceResult =
  | Exclude<WorkspaceState, { status: "loading" }>
  | { status: "unauthenticated" };

function isInvalidSession(error: unknown): boolean {
  return error instanceof ApiRequestError && error.code === "invalid_session";
}

function toWorkspaceError(
  error: unknown,
): Extract<WorkspaceState, { status: "error" }> {
  if (error instanceof ApiRequestError) {
    return {
      status: "error",
      message: error.message,
      requestId: error.requestId,
    };
  }
  return { status: "error", message: "工作台加载失败，请稍后重试。" };
}

async function readWorkspace(): Promise<WorkspaceResult> {
  try {
    return { status: "ready", workspace: await getIdentityWorkspace() };
  } catch (error) {
    return isInvalidSession(error)
      ? { status: "unauthenticated" }
      : toWorkspaceError(error);
  }
}

export function EventsWorkspace() {
  const router = useRouter();
  const [state, setState] = useState<WorkspaceState>({ status: "loading" });
  const [isSigningOut, setIsSigningOut] = useState(false);
  const [signOutError, setSignOutError] = useState<string | null>(null);

  useEffect(() => {
    let isCurrent = true;
    void readWorkspace().then((result) => {
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

  async function reloadWorkspace() {
    setState({ status: "loading" });
    const result = await readWorkspace();
    if (result.status === "unauthenticated") {
      router.replace("/login");
      return;
    }
    setState(result);
  }

  async function signOut() {
    if (isSigningOut) {
      return;
    }
    setIsSigningOut(true);
    setSignOutError(null);
    try {
      await deleteIdentitySession();
      router.replace("/login");
      router.refresh();
    } catch (error) {
      if (isInvalidSession(error)) {
        router.replace("/login");
        return;
      }
      setSignOutError(
        error instanceof ApiRequestError ? error.message : "注销失败，请重试。",
      );
    } finally {
      setIsSigningOut(false);
    }
  }

  if (state.status === "loading") {
    return (
      <PageState
        eyebrow="工作台"
        title="正在加载"
        description="正在验证会话并读取你的私有工作区。"
      />
    );
  }

  if (state.status === "error") {
    return (
      <PageState
        eyebrow="加载失败"
        title="暂时无法打开工作台"
        description={
          state.requestId
            ? `${state.message} 请求编号：${state.requestId}`
            : state.message
        }
        action={
          <Button onClick={() => void reloadWorkspace()}>
            <RotateCcwIcon data-icon="inline-start" />
            重新加载
          </Button>
        }
      />
    );
  }

  return (
    <div className="bg-background min-h-screen">
      <header className="mx-auto flex h-16 max-w-7xl items-center justify-between px-5 sm:px-8 xl:px-16 2xl:px-0">
        <BrandLockup href="/events" compactOnMobile />
        <div className="flex items-center gap-2">
          <Button
            asChild
            variant="ghost"
            size="icon"
            className="sm:h-11 sm:w-auto sm:px-3 md:h-8 md:px-2.5"
          >
            <Link href="/content">
              <FileTextIcon className="sm:hidden" aria-hidden="true" />
              <span className="sr-only sm:not-sr-only">作品资料</span>
            </Link>
          </Button>
          <Button
            asChild
            variant="ghost"
            size="icon"
            className="sm:h-11 sm:w-auto sm:px-3 md:h-8 md:px-2.5"
          >
            <Link href="/sources">
              <RadioIcon className="sm:hidden" aria-hidden="true" />
              <span className="sr-only sm:not-sr-only">来源状态</span>
            </Link>
          </Button>
          <Button
            asChild
            variant="ghost"
            size="icon"
            className="sm:h-11 sm:w-auto sm:px-3 md:h-8 md:px-2.5"
          >
            <Link href="/jobs">
              <ListChecksIcon className="sm:hidden" aria-hidden="true" />
              <span className="sr-only sm:not-sr-only">任务记录</span>
            </Link>
          </Button>
          <Button
            asChild
            variant="ghost"
            size="icon"
            className="sm:h-11 sm:w-auto sm:px-3 md:h-8 md:px-2.5"
          >
            <Link href="/reports">
              <NewspaperIcon className="sm:hidden" aria-hidden="true" />
              <span className="sr-only sm:not-sr-only">日报</span>
            </Link>
          </Button>
          <span className="text-muted-foreground hidden text-sm sm:inline">
            {state.workspace.owner.username}
          </span>
          <Button
            variant="ghost"
            size="navigation"
            onClick={() => void signOut()}
            disabled={isSigningOut}
          >
            <LogOutIcon data-icon="inline-start" />
            {isSigningOut ? "正在退出" : "退出"}
          </Button>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-5 py-12 sm:px-8 sm:py-16 xl:px-16 2xl:px-0">
        <div className="flex flex-col gap-8 sm:flex-row sm:items-end sm:justify-between">
          <div className="max-w-2xl">
            <h1 className="text-4xl font-semibold tracking-tight text-balance sm:text-5xl">
              从关键词开始关注
            </h1>
            <p className="text-muted-foreground mt-5 leading-7">
              创建你关心的品牌、产品或话题，按可用来源汇集相关讨论。
            </p>
          </div>
          <Button asChild size="lg">
            <Link href="/monitors/new">
              <PlusIcon data-icon="inline-start" />
              新建监控主题
            </Link>
          </Button>
        </div>
        <TopicList />
        <section className="border-border mt-24 border-t pt-8">
          <h2 className="text-xl font-semibold tracking-tight">事件脉络</h2>
          <p className="text-muted-foreground mt-3 max-w-2xl text-sm leading-7">
            事件归并仍在建设中。后续可沿时间与来源查看同一主题下的事件发展。
          </p>
        </section>
        {signOutError ? (
          <p role="alert" className="text-destructive mt-4 text-sm">
            {signOutError}
          </p>
        ) : null}
      </main>
    </div>
  );
}
