"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useEffect, useState } from "react";
import { ArrowLeftIcon, ArrowRightIcon, LoaderCircleIcon } from "lucide-react";

import { getIdentityWorkspace } from "@/api/identity";
import { createMonitorTopic } from "@/api/jiankongzhuti";
import { listSourceCapabilities } from "@/api/laiyuannengli";
import {
  KeywordGroupField,
  parseKeywordLines,
} from "@/components/monitors/keyword-group-field";
import { TopicRulePreview } from "@/components/monitors/topic-rule-preview";
import {
  parseNotificationTargetNames,
  selectableTopicSources,
  TopicSettingsFields,
  type TopicSourceOption,
} from "@/components/monitors/topic-settings-fields";
import { PageState } from "@/components/system/page-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiRequestError } from "@/request";

type SubmissionError = {
  message: string;
  requestId?: string;
};

type AccessState =
  | { status: "checking" }
  | { status: "ready"; sourceOptions: TopicSourceOption[] }
  | { status: "error"; message: string; requestId?: string };

function toSubmissionError(error: unknown): SubmissionError {
  if (error instanceof ApiRequestError) {
    if (error.code === "keyword_group_conflict") {
      return { message: "同一个关键词不能同时放在包含组与排除组中。" };
    }
    if (error.code === "invalid_monitor_rules") {
      return { message: "至少填写一个“任意命中”或“全部包含”关键词。" };
    }
    if (error.code === "source_preset_not_applied") {
      return { message: "所选来源尚未应用预设，或不支持关键词搜索。" };
    }
    return { message: error.message, requestId: error.requestId };
  }
  return { message: "主题保存失败，请稍后重试。" };
}

type ExpectedTopicSettingsInput = {
  source_keys: string[];
  collection_interval_seconds: number;
  report_time: string;
  weekly_report_enabled: boolean;
  notification_target_names: string[];
};

export function TopicForm() {
  const router = useRouter();
  const [accessState, setAccessState] = useState<AccessState>({
    status: "checking",
  });
  const [name, setName] = useState("");
  const [matchAny, setMatchAny] = useState("");
  const [matchAll, setMatchAll] = useState("");
  const [exclude, setExclude] = useState("");
  const [sourceKeys, setSourceKeys] = useState<string[]>([]);
  const [collectionIntervalSeconds, setCollectionIntervalSeconds] =
    useState(1800);
  const [reportTime, setReportTime] = useState("09:00");
  const [weeklyReportEnabled, setWeeklyReportEnabled] = useState(false);
  const [notificationTargets, setNotificationTargets] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submissionError, setSubmissionError] =
    useState<SubmissionError | null>(null);

  useEffect(() => {
    let isCurrent = true;
    void Promise.all([getIdentityWorkspace(), listSourceCapabilities()])
      .then(([, sourcePage]) => {
        if (isCurrent) {
          setAccessState({
            status: "ready",
            sourceOptions: selectableTopicSources(sourcePage.items),
          });
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
        } else if (error instanceof ApiRequestError) {
          setAccessState({
            status: "error",
            message: error.message,
            requestId: error.requestId,
          });
        } else {
          setAccessState({
            status: "error",
            message: "无法验证当前会话，请稍后重试。",
          });
        }
      });
    return () => {
      isCurrent = false;
    };
  }, [router]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmitting) {
      return;
    }
    const any = parseKeywordLines(matchAny);
    const all = parseKeywordLines(matchAll);
    if (any.length === 0 && all.length === 0) {
      setSubmissionError({
        message: "至少填写一个“任意命中”或“全部包含”关键词。",
      });
      return;
    }
    const targetNames = parseNotificationTargetNames(notificationTargets);
    if (
      !Number.isInteger(collectionIntervalSeconds) ||
      collectionIntervalSeconds < 600 ||
      collectionIntervalSeconds > 86400
    ) {
      setSubmissionError({
        message: "采集频率必须是 600—86400 之间的整数秒。",
      });
      return;
    }
    if (
      targetNames.length > 20 ||
      targetNames.some((item) => item.length > 128)
    ) {
      setSubmissionError({
        message: "推送目标最多 20 个，每个名称不超过 128 个字符。",
      });
      return;
    }

    setIsSubmitting(true);
    setSubmissionError(null);
    try {
      const payload: HotKeyAPI.MonitorTopicCreateInput &
        ExpectedTopicSettingsInput = {
        name,
        match_any: any,
        match_all: all,
        exclude: parseKeywordLines(exclude),
        source_keys: sourceKeys,
        collection_interval_seconds: collectionIntervalSeconds,
        report_time: reportTime,
        weekly_report_enabled: weeklyReportEnabled,
        notification_target_names: targetNames,
      };
      const topic = await createMonitorTopic(payload);
      router.replace(`/monitors/${topic.id}`);
      router.refresh();
    } catch (error) {
      if (
        error instanceof ApiRequestError &&
        error.code === "invalid_session"
      ) {
        router.replace("/login");
      } else {
        setSubmissionError(toSubmissionError(error));
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  if (accessState.status === "checking") {
    return (
      <PageState
        eyebrow="监控主题"
        title="正在准备创建页"
        description="正在验证会话与私有工作台边界。"
      />
    );
  }

  if (accessState.status === "error") {
    return (
      <PageState
        eyebrow="加载失败"
        title="暂时无法打开创建页"
        description={
          accessState.requestId
            ? `${accessState.message} 请求编号：${accessState.requestId}`
            : accessState.message
        }
        action={
          <Button type="button" onClick={() => window.location.reload()}>
            重新加载
          </Button>
        }
      />
    );
  }

  return (
    <div className="bg-background min-h-screen">
      <header className="mx-auto flex h-16 max-w-5xl items-center px-5 sm:px-8 xl:px-0">
        <Button asChild variant="ghost" size="navigation">
          <Link href="/events">
            <ArrowLeftIcon data-icon="inline-start" />
            返回工作台
          </Link>
        </Button>
      </header>

      <main className="mx-auto max-w-5xl px-5 py-10 sm:px-8 sm:py-14 xl:px-0">
        <Badge variant="secondary">监控主题</Badge>
        <h1 className="mt-5 text-3xl font-semibold tracking-tight sm:text-4xl">
          新建主题
        </h1>
        <p className="text-muted-foreground mt-3 max-w-2xl text-sm leading-6">
          先保存可解释的本地规则。来源尚未选择，主题会保持暂停；保存不会发起采集。
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
                  disabled={isSubmitting}
                  minLength={1}
                  maxLength={80}
                  required
                  autoFocus
                  placeholder="例如：品牌召回"
                />
                <p className="text-muted-foreground text-xs leading-5">
                  名称用于辨认主题，允许重名；规则变化才会生成新版本。
                </p>
              </div>
            </section>

            <section className="space-y-6">
              <KeywordGroupField
                id="match-any"
                label="任意命中"
                description="其中任意一个关键词出现即可；与“全部包含”同时填写时，两组条件都要满足。"
                value={matchAny}
                onChange={setMatchAny}
                disabled={isSubmitting}
              />
              <KeywordGroupField
                id="match-all"
                label="全部包含"
                description="这里的每个关键词都必须出现。该组为空时不会额外限制。"
                value={matchAll}
                onChange={setMatchAll}
                disabled={isSubmitting}
              />
              <KeywordGroupField
                id="exclude"
                label="排除"
                description="任一排除词命中都会优先剔除结果。不要与包含组填写相同关键词。"
                value={exclude}
                onChange={setExclude}
                disabled={isSubmitting}
              />
            </section>

            <TopicSettingsFields
              sourceOptions={accessState.sourceOptions}
              sourceKeys={sourceKeys}
              onSourceKeysChange={setSourceKeys}
              collectionIntervalSeconds={collectionIntervalSeconds}
              onCollectionIntervalSecondsChange={setCollectionIntervalSeconds}
              reportTime={reportTime}
              onReportTimeChange={setReportTime}
              weeklyReportEnabled={weeklyReportEnabled}
              onWeeklyReportEnabledChange={setWeeklyReportEnabled}
              notificationTargets={notificationTargets}
              onNotificationTargetsChange={setNotificationTargets}
              disabled={isSubmitting}
            />
          </div>

          <aside className="lg:sticky lg:top-8 lg:self-start">
            <div className="bg-muted rounded-2xl p-5">
              <p className="text-sm font-medium">保存后的状态</p>
              <dl className="text-muted-foreground mt-4 space-y-3 text-sm">
                <div className="flex justify-between gap-4">
                  <dt>运行状态</dt>
                  <dd className="text-foreground">已暂停</dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt>来源</dt>
                  <dd className="text-foreground">
                    {sourceKeys.length > 0
                      ? `${sourceKeys.length} 个`
                      : "待选择"}
                  </dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt>规则版本</dt>
                  <dd className="text-foreground">v1</dd>
                </div>
              </dl>
              {submissionError ? (
                <div
                  role="alert"
                  className="bg-destructive/10 text-foreground mt-5 rounded-lg px-3 py-2.5 text-sm leading-5"
                >
                  <p>{submissionError.message}</p>
                  {submissionError.requestId ? (
                    <p className="mt-1 font-mono text-xs opacity-80">
                      请求编号：{submissionError.requestId}
                    </p>
                  ) : null}
                </div>
              ) : null}
              <div className="mt-5">
                <TopicRulePreview
                  matchAny={matchAny}
                  matchAll={matchAll}
                  exclude={exclude}
                  disabled={isSubmitting}
                />
              </div>
              <Button className="mt-5 w-full" size="lg" disabled={isSubmitting}>
                {isSubmitting ? (
                  <LoaderCircleIcon
                    className="animate-spin"
                    aria-hidden="true"
                  />
                ) : (
                  <ArrowRightIcon data-icon="inline-end" />
                )}
                {isSubmitting ? "正在保存" : "保存主题"}
              </Button>
            </div>
          </aside>
        </form>
      </main>
    </div>
  );
}
