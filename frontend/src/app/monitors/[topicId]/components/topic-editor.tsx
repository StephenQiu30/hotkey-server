"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useCallback, useEffect, useState } from "react";
import {
  ArrowLeftIcon,
  ArchiveIcon,
  CheckCircle2Icon,
  CopyIcon,
  LoaderCircleIcon,
  PauseIcon,
  PlayIcon,
  RefreshCwIcon,
  RotateCcwIcon,
  SaveIcon,
} from "lucide-react";

import {
  archiveMonitorTopic,
  cloneMonitorTopic,
  getMonitorTopic,
  pauseMonitorTopic,
  resumeMonitorTopic,
  updateMonitorTopic,
} from "@/api/jiankongzhuti";
import {
  KeywordGroupField,
  parseKeywordLines,
} from "@/components/monitors/keyword-group-field";
import { PageState } from "@/components/system/page-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiRequestError } from "@/request";

type TopicEditorProps = { topicId: string };

type EditorState =
  | { status: "loading" }
  | { status: "ready"; topic: HotKeyAPI.MonitorTopicView }
  | { status: "not-found" }
  | { status: "error"; message: string; requestId?: string };

type ActionFeedback = {
  kind: "error" | "conflict" | "success";
  message: string;
  requestId?: string;
};

type PendingAction = "archive" | "clone" | "pause" | "resume" | "save";

function isInvalidSession(error: unknown): boolean {
  return error instanceof ApiRequestError && error.code === "invalid_session";
}

function toActionFeedback(error: unknown): ActionFeedback {
  if (error instanceof ApiRequestError) {
    if (error.code === "topic_version_conflict") {
      return {
        kind: "conflict",
        message: "这个主题已在其他页面更新。重新读取后再确认你的修改。",
      };
    }
    if (error.code === "keyword_group_conflict") {
      return {
        kind: "error",
        message: "同一个关键词不能同时放在包含组与排除组中。",
      };
    }
    return {
      kind: "error",
      message: error.message,
      requestId: error.requestId,
    };
  }
  return { kind: "error", message: "主题操作失败，请稍后重试。" };
}

export function TopicEditor({ topicId }: TopicEditorProps) {
  const router = useRouter();
  const [state, setState] = useState<EditorState>({ status: "loading" });
  const [name, setName] = useState("");
  const [matchAny, setMatchAny] = useState("");
  const [matchAll, setMatchAll] = useState("");
  const [exclude, setExclude] = useState("");
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(
    null,
  );
  const [feedback, setFeedback] = useState<ActionFeedback | null>(null);
  const isBusy = pendingAction !== null;

  const applyTopic = useCallback((topic: HotKeyAPI.MonitorTopicView) => {
    setState({ status: "ready", topic });
    setName(topic.name);
    setMatchAny(topic.rules.match_any.join("\n"));
    setMatchAll(topic.rules.match_all.join("\n"));
    setExclude(topic.rules.exclude.join("\n"));
  }, []);

  const loadTopic = useCallback(async () => {
    setFeedback(null);
    try {
      applyTopic(await getMonitorTopic({ topic_id: topicId }));
    } catch (error) {
      if (isInvalidSession(error)) {
        router.replace("/login");
      } else if (
        error instanceof ApiRequestError &&
        error.code === "resource_not_found"
      ) {
        setState({ status: "not-found" });
      } else if (error instanceof ApiRequestError) {
        setState({
          status: "error",
          message: error.message,
          requestId: error.requestId,
        });
      } else {
        setState({ status: "error", message: "主题加载失败，请稍后重试。" });
      }
    }
  }, [applyTopic, router, topicId]);

  useEffect(() => {
    let isCurrent = true;
    void getMonitorTopic({ topic_id: topicId })
      .then((topic) => {
        if (isCurrent) {
          applyTopic(topic);
        }
      })
      .catch((error: unknown) => {
        if (!isCurrent) {
          return;
        }
        if (isInvalidSession(error)) {
          router.replace("/login");
        } else if (
          error instanceof ApiRequestError &&
          error.code === "resource_not_found"
        ) {
          setState({ status: "not-found" });
        } else if (error instanceof ApiRequestError) {
          setState({
            status: "error",
            message: error.message,
            requestId: error.requestId,
          });
        } else {
          setState({
            status: "error",
            message: "主题加载失败，请稍后重试。",
          });
        }
      });
    return () => {
      isCurrent = false;
    };
  }, [applyTopic, router, topicId]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (state.status !== "ready" || isBusy) {
      return;
    }
    const any = parseKeywordLines(matchAny);
    const all = parseKeywordLines(matchAll);
    if (any.length === 0 && all.length === 0) {
      setFeedback({
        kind: "error",
        message: "至少填写一个“任意命中”或“全部包含”关键词。",
      });
      return;
    }

    setPendingAction("save");
    setFeedback(null);
    try {
      const topic = await updateMonitorTopic(
        { topic_id: topicId },
        {
          name,
          match_any: any,
          match_all: all,
          exclude: parseKeywordLines(exclude),
          expected_version: state.topic.current_version,
        },
      );
      applyTopic(topic);
      setFeedback({
        kind: "success",
        message: `已保存。当前规则版本为 v${topic.current_version}。`,
      });
    } catch (error) {
      if (isInvalidSession(error)) {
        router.replace("/login");
      } else {
        setFeedback(toActionFeedback(error));
      }
    } finally {
      setPendingAction(null);
    }
  }

  async function runLifecycleAction(
    action: "archive" | "clone" | "pause" | "resume",
  ) {
    if (state.status !== "ready" || isBusy) {
      return;
    }
    setPendingAction(action);
    setFeedback(null);
    try {
      if (action === "clone") {
        const clone = await cloneMonitorTopic({ topic_id: topicId });
        router.push(`/monitors/${clone.id}`);
        return;
      }
      const operation =
        action === "archive"
          ? archiveMonitorTopic
          : action === "pause"
            ? pauseMonitorTopic
            : resumeMonitorTopic;
      const topic = await operation({ topic_id: topicId });
      applyTopic(topic);
      setFeedback({
        kind: "success",
        message:
          action === "archive"
            ? "主题已归档，规则历史仍会保留。"
            : action === "pause"
              ? "主题已暂停；正在运行的任务需在任务详情单独取消。"
              : "主题已恢复。",
      });
    } catch (error) {
      if (isInvalidSession(error)) {
        router.replace("/login");
      } else {
        setFeedback(toActionFeedback(error));
      }
    } finally {
      setPendingAction(null);
    }
  }

  if (state.status === "loading") {
    return (
      <PageState
        eyebrow="监控主题"
        title="正在读取主题"
        description="正在读取当前规则版本与持久状态。"
      />
    );
  }
  if (state.status === "not-found") {
    return (
      <PageState
        eyebrow="主题不可用"
        title="没有找到这个主题"
        description="主题不存在，或当前使用者无权查看。"
        action={
          <Button asChild variant="secondary">
            <Link href="/events">返回工作台</Link>
          </Button>
        }
      />
    );
  }
  if (state.status === "error") {
    return (
      <PageState
        eyebrow="加载失败"
        title="暂时无法读取主题"
        description={
          state.requestId
            ? `${state.message} 请求编号：${state.requestId}`
            : state.message
        }
        action={
          <Button type="button" onClick={() => void loadTopic()}>
            <RotateCcwIcon data-icon="inline-start" />
            重新加载
          </Button>
        }
      />
    );
  }

  const { topic } = state;
  return (
    <div className="bg-background min-h-screen">
      <header className="mx-auto flex h-16 max-w-5xl items-center justify-between px-5 sm:px-8 xl:px-0">
        <Button asChild variant="ghost" size="navigation">
          <Link href="/events">
            <ArrowLeftIcon data-icon="inline-start" />
            返回工作台
          </Link>
        </Button>
        <span className="text-muted-foreground hidden font-mono text-xs sm:inline">
          {topic.id}
        </span>
      </header>

      <main className="mx-auto max-w-5xl px-5 py-10 sm:px-8 sm:py-14 xl:px-0">
        <div className="flex flex-wrap items-center gap-2">
          <Badge
            variant={topic.status === "archived" ? "outline" : "secondary"}
          >
            {topic.status === "archived"
              ? "已归档"
              : topic.status === "active"
                ? "运行中"
                : "已暂停"}
          </Badge>
          <Badge variant="outline">
            {topic.readiness_status === "ready" ? "来源已就绪" : "待选择来源"}
          </Badge>
          <span className="text-muted-foreground text-sm">
            规则版本 v{topic.current_version}
          </span>
        </div>
        <h1 className="mt-5 text-3xl font-semibold tracking-tight sm:text-4xl">
          编辑主题
        </h1>
        <p className="text-muted-foreground mt-3 max-w-2xl text-sm leading-6">
          规则变化会保留新版本；只修改名称不会创建空规则版本。保存仍不会发起采集。
        </p>

        <form
          className="mt-10 grid gap-8 lg:grid-cols-[minmax(0,1fr)_18rem]"
          onSubmit={handleSubmit}
        >
          <div className="space-y-8">
            <section className="bg-muted rounded-2xl p-5 sm:p-7">
              <div className="space-y-2">
                <Label htmlFor="topic-name">主题名称</Label>
                <Input
                  id="topic-name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  disabled={isBusy || topic.status === "archived"}
                  minLength={1}
                  maxLength={80}
                  required
                />
              </div>
            </section>

            <section className="space-y-6">
              <KeywordGroupField
                id="match-any"
                label="任意命中"
                description="其中任意一个关键词出现即可；与“全部包含”同时填写时，两组条件都要满足。"
                value={matchAny}
                onChange={setMatchAny}
                disabled={isBusy || topic.status === "archived"}
              />
              <KeywordGroupField
                id="match-all"
                label="全部包含"
                description="这里的每个关键词都必须出现。该组为空时不会额外限制。"
                value={matchAll}
                onChange={setMatchAll}
                disabled={isBusy || topic.status === "archived"}
              />
              <KeywordGroupField
                id="exclude"
                label="排除"
                description="任一排除词命中都会优先剔除结果。不要与包含组填写相同关键词。"
                value={exclude}
                onChange={setExclude}
                disabled={isBusy || topic.status === "archived"}
              />
            </section>
          </div>

          <aside className="lg:sticky lg:top-8 lg:self-start">
            <div className="bg-muted rounded-2xl p-5">
              <p className="text-sm font-medium">当前配置</p>
              <dl className="text-muted-foreground mt-4 space-y-3 text-sm">
                <div className="flex justify-between gap-4">
                  <dt>运行状态</dt>
                  <dd className="text-foreground">
                    {topic.status === "archived"
                      ? "已归档"
                      : topic.status === "active"
                        ? "运行中"
                        : "已暂停"}
                  </dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt>来源</dt>
                  <dd className="text-foreground">
                    {topic.readiness_status === "ready" ? "已就绪" : "待选择"}
                  </dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt>当前版本</dt>
                  <dd className="text-foreground">v{topic.current_version}</dd>
                </div>
              </dl>

              {feedback ? (
                <div
                  role={feedback.kind === "success" ? "status" : "alert"}
                  className={
                    feedback.kind === "success"
                      ? "mt-5 rounded-lg bg-emerald-500/10 px-3 py-2.5 text-sm leading-5 text-emerald-700 dark:text-emerald-300"
                      : "bg-destructive/10 text-foreground mt-5 rounded-lg px-3 py-2.5 text-sm leading-5"
                  }
                >
                  <p className="flex items-start gap-2">
                    {feedback.kind === "success" ? (
                      <CheckCircle2Icon className="mt-0.5 size-4 shrink-0" />
                    ) : null}
                    <span>{feedback.message}</span>
                  </p>
                  {feedback.requestId ? (
                    <p className="mt-1 font-mono text-xs opacity-80">
                      请求编号：{feedback.requestId}
                    </p>
                  ) : null}
                  {feedback.kind === "conflict" ? (
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      className="mt-3"
                      onClick={() => void loadTopic()}
                    >
                      <RefreshCwIcon data-icon="inline-start" />
                      重新读取
                    </Button>
                  ) : null}
                </div>
              ) : null}

              <Button
                className="mt-5 w-full"
                size="lg"
                disabled={isBusy || topic.status === "archived"}
              >
                {pendingAction === "save" ? (
                  <LoaderCircleIcon
                    className="animate-spin"
                    aria-hidden="true"
                  />
                ) : (
                  <SaveIcon data-icon="inline-start" />
                )}
                {pendingAction === "save" ? "正在保存" : "保存修改"}
              </Button>

              <div className="mt-3 grid grid-cols-2 gap-2">
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  disabled={isBusy}
                  onClick={() => void runLifecycleAction("clone")}
                >
                  <CopyIcon data-icon="inline-start" />
                  复制
                </Button>
                {topic.status === "active" ? (
                  <Button
                    type="button"
                    variant="secondary"
                    size="sm"
                    disabled={isBusy}
                    onClick={() => void runLifecycleAction("pause")}
                  >
                    <PauseIcon data-icon="inline-start" />
                    暂停
                  </Button>
                ) : topic.status === "paused" ? (
                  <Button
                    type="button"
                    variant="secondary"
                    size="sm"
                    disabled={isBusy}
                    onClick={() => void runLifecycleAction("resume")}
                  >
                    <PlayIcon data-icon="inline-start" />
                    恢复
                  </Button>
                ) : null}
              </div>
              {topic.status !== "archived" ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="mt-2 w-full"
                  disabled={isBusy}
                  onClick={() => void runLifecycleAction("archive")}
                >
                  <ArchiveIcon data-icon="inline-start" />
                  归档主题
                </Button>
              ) : null}
            </div>
          </aside>
        </form>
      </main>
    </div>
  );
}
