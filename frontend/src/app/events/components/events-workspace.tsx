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
      <header className="mx-auto flex h-16 max-w-7xl items-center justify-between px-5 sm:px-8 xl:px-0">
        <Link href="/events" className="flex items-center gap-2.5 font-medium">
          <span className="bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-lg font-mono text-xs">
            HK
          </span>
          <span className="hidden sm:inline">HotKey</span>
        </Link>
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

      <main className="mx-auto max-w-7xl px-5 py-12 sm:px-8 sm:py-16 xl:px-0">
        <p className="text-muted-foreground font-mono text-xs tracking-wider uppercase">
          Events
        </p>
        <div className="mt-3 flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
          <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
            事件
          </h1>
          <Button asChild>
            <Link href="/monitors/new">
              <PlusIcon data-icon="inline-start" />
              新建监控主题
            </Link>
          </Button>
        </div>
        <section className="bg-muted mt-10 rounded-2xl px-6 py-16 text-center sm:px-10 sm:py-24">
          <h2 className="text-xl font-medium">尚无事件</h2>
          <p className="text-muted-foreground mx-auto mt-3 max-w-md text-sm leading-6">
            完成监控配置后，发现的热点事件会汇总到这里。
          </p>
        </section>
        <TopicList />
        {signOutError ? (
          <p role="alert" className="text-destructive mt-4 text-sm">
            {signOutError}
          </p>
        ) : null}
      </main>
    </div>
  );
}
