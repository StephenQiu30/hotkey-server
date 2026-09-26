"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ArchiveIcon, LoaderCircleIcon, RotateCcwIcon } from "lucide-react";

import { listMonitorTopics } from "@/api/jiankongzhuti";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ApiRequestError } from "@/request";

type TopicListState =
  | { status: "loading" }
  | { status: "ready"; topics: HotKeyAPI.MonitorTopicView[] }
  | { status: "error"; message: string; requestId?: string };

export function TopicList() {
  const router = useRouter();
  const [includeArchived, setIncludeArchived] = useState(false);
  const [state, setState] = useState<TopicListState>({ status: "loading" });

  async function load() {
    setState({ status: "loading" });
    try {
      const page = await listMonitorTopics({
        include_archived: includeArchived,
        limit: 50,
      });
      setState({ status: "ready", topics: page.items });
    } catch (error) {
      if (
        error instanceof ApiRequestError &&
        error.code === "invalid_session"
      ) {
        router.replace("/login");
        return;
      }
      setState({
        status: "error",
        message:
          error instanceof ApiRequestError
            ? error.message
            : "主题列表加载失败。",
        requestId:
          error instanceof ApiRequestError ? error.requestId : undefined,
      });
    }
  }

  useEffect(() => {
    let isCurrent = true;
    void listMonitorTopics({ include_archived: includeArchived, limit: 50 })
      .then((page) => {
        if (isCurrent) {
          setState({ status: "ready", topics: page.items });
        }
      })
      .catch((error: unknown) => {
        if (!isCurrent) {
          return;
        }
        if (
          error instanceof ApiRequestError &&
          error.code === "invalid_session"
        ) {
          router.replace("/login");
          return;
        }
        setState({
          status: "error",
          message:
            error instanceof ApiRequestError
              ? error.message
              : "主题列表加载失败。",
          requestId:
            error instanceof ApiRequestError ? error.requestId : undefined,
        });
      });
    return () => {
      isCurrent = false;
    };
  }, [includeArchived, router]);

  return (
    <section className="mt-10" aria-labelledby="monitor-topics-heading">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 id="monitor-topics-heading" className="text-xl font-semibold">
            监控主题
          </h2>
        </div>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => setIncludeArchived((value) => !value)}
        >
          <ArchiveIcon data-icon="inline-start" />
          {includeArchived ? "隐藏已归档" : "显示已归档"}
        </Button>
      </div>

      {state.status === "loading" ? (
        <div className="text-muted-foreground mt-5 flex items-center gap-2 text-sm">
          <LoaderCircleIcon
            className="size-4 animate-spin"
            aria-hidden="true"
          />
          正在读取主题
        </div>
      ) : null}

      {state.status === "error" ? (
        <div
          className="bg-destructive/10 mt-5 rounded-xl p-4 text-sm"
          role="alert"
        >
          <p>{state.message}</p>
          {state.requestId ? (
            <p className="mt-1 font-mono text-xs">
              请求编号：{state.requestId}
            </p>
          ) : null}
          <Button
            type="button"
            variant="secondary"
            size="sm"
            className="mt-3"
            onClick={() => void load()}
          >
            <RotateCcwIcon data-icon="inline-start" />
            重新加载
          </Button>
        </div>
      ) : null}

      {state.status === "ready" && state.topics.length === 0 ? (
        <div className="bg-muted mt-5 rounded-2xl px-6 py-10 text-center">
          <p className="font-medium">尚无监控主题</p>
          <p className="text-muted-foreground mt-2 text-sm">
            先保存本地规则，保存不会发起采集。
          </p>
        </div>
      ) : null}

      {state.status === "ready" && state.topics.length > 0 ? (
        <div className="mt-6 grid gap-5 md:grid-cols-2">
          {state.topics.map((topic) => (
            <Link
              key={topic.id}
              href={`/monitors/${topic.id}`}
              className="bg-muted hover:bg-accent focus-visible:ring-ring rounded-2xl p-7 transition-colors focus-visible:ring-2 focus-visible:outline-none"
            >
              <Badge
                variant={topic.status === "archived" ? "outline" : "secondary"}
              >
                {topic.status === "archived"
                  ? "已归档"
                  : topic.status === "active"
                    ? "运行中"
                    : "已暂停"}
              </Badge>
              <h3 className="mt-8 text-2xl font-semibold tracking-tight">
                {topic.name}
              </h3>
              <p className="text-muted-foreground mt-5 text-sm">
                查看关键词与来源 →
              </p>
            </Link>
          ))}
        </div>
      ) : null}
    </section>
  );
}
