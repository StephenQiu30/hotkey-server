"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useEffect, useRef, useState } from "react";
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
import {
  readTopicFieldErrors,
  topicErrorAction,
  tryBeginTopicSubmission,
  type TopicFieldErrors,
} from "@/components/monitors/topic-validation";
import { PageState } from "@/components/system/page-state";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ApiRequestError } from "@/request";

type SubmissionError = {
  message: string;
  requestId?: string;
  fields?: TopicFieldErrors;
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
      return { message: "所选来源缺少已应用搜索预设、准入或启用的执行策略。" };
    }
    return {
      message: error.message,
      requestId: error.requestId,
      fields: readTopicFieldErrors(error),
    };
  }
  return { message: "主题保存失败，请稍后重试。" };
}

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
  const submittingRef = useRef(false);
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
        if (topicErrorAction(error) === "login") {
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
    if (!tryBeginTopicSubmission(submittingRef)) {
      return;
    }
    const any = parseKeywordLines(matchAny);
    const all = parseKeywordLines(matchAll);
    if (any.length === 0 && all.length === 0) {
      setSubmissionError({
        message: "至少填写一个“任意命中”或“全部包含”关键词。",
        fields: { match_any: "至少填写一个包含关键词。" },
      });
      submittingRef.current = false;
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
        fields: {
          collection_interval_seconds: "请输入 600—86400 之间的整数秒。",
        },
      });
      submittingRef.current = false;
      return;
    }
    if (
      targetNames.length > 20 ||
      targetNames.some((item) => item.length > 128)
    ) {
      setSubmissionError({
        message: "推送目标最多 20 个，每个名称不超过 128 个字符。",
      });
      submittingRef.current = false;
      return;
    }

    setIsSubmitting(true);
    setSubmissionError(null);
    try {
      const payload: HotKeyAPI.MonitorTopicCreateInput = {
        name,
        match_any: any,
        match_all: all,
        exclude: parseKeywordLines(exclude),
        source_keys: sourceKeys,
        collection_interval_seconds: collectionIntervalSeconds,
        report_time: reportTime || "09:00",
        weekly_report_enabled: weeklyReportEnabled,
        notification_target_names: targetNames,
      };
      const topic = await createMonitorTopic(payload);
      router.replace(`/monitors/${topic.id}`);
      router.refresh();
    } catch (error) {
      if (topicErrorAction(error) === "login") {
        router.replace("/login");
      } else {
        setSubmissionError(toSubmissionError(error));
      }
    } finally {
      submittingRef.current = false;
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
          先保存可解释的本地规则。新主题保持暂停；保存不会发起采集。
        </p>

        <form
          className="mt-10 grid gap-8 lg:grid-cols-3"
          onSubmit={handleSubmit}
        >
          <div className="flex flex-col gap-8 lg:col-span-2">
            <Card className="bg-muted rounded-2xl py-5 sm:py-7">
              <CardContent className="px-5 sm:px-7">
                <Field
                  data-disabled={isSubmitting}
                  data-invalid={Boolean(submissionError?.fields?.name)}
                >
                  <FieldLabel htmlFor="topic-name">主题名称</FieldLabel>
                  <Input
                    id="topic-name"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    disabled={isSubmitting}
                    minLength={1}
                    maxLength={80}
                    required
                    aria-invalid={Boolean(submissionError?.fields?.name)}
                    autoFocus
                    placeholder="例如：品牌召回"
                  />
                  <FieldDescription>
                    名称用于辨认主题，允许重名；采集规则或来源设置变化才会生成新版本。
                  </FieldDescription>
                  {submissionError?.fields?.name ? (
                    <FieldError>{submissionError.fields.name}</FieldError>
                  ) : null}
                </Field>
              </CardContent>
            </Card>

            <Card className="gap-6 rounded-2xl py-5 sm:py-7">
              <CardHeader className="px-5 sm:px-7">
                <CardTitle asChild>
                  <h2>匹配规则</h2>
                </CardTitle>
                <CardDescription>用关键词界定需要关注的讨论。</CardDescription>
              </CardHeader>
              <CardContent className="px-5 sm:px-7">
                <FieldGroup className="gap-6">
                  <KeywordGroupField
                    id="match-any"
                    label="任意命中"
                    description="其中任意一个关键词出现即可；与“全部包含”同时填写时，两组条件都要满足。"
                    value={matchAny}
                    onChange={setMatchAny}
                    disabled={isSubmitting}
                    error={submissionError?.fields?.match_any}
                  />
                  <KeywordGroupField
                    id="match-all"
                    label="全部包含"
                    description="这里的每个关键词都必须出现。该组为空时不会额外限制。"
                    value={matchAll}
                    onChange={setMatchAll}
                    disabled={isSubmitting}
                    error={submissionError?.fields?.match_all}
                  />
                  <KeywordGroupField
                    id="exclude"
                    label="排除"
                    description="任一排除词命中都会优先剔除结果。不要与包含组填写相同关键词。"
                    value={exclude}
                    onChange={setExclude}
                    disabled={isSubmitting}
                    error={submissionError?.fields?.exclude}
                  />
                </FieldGroup>
              </CardContent>
            </Card>

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
              fieldErrors={submissionError?.fields}
            />
          </div>

          <aside className="lg:sticky lg:top-8 lg:self-start">
            <Card className="bg-muted gap-0 rounded-2xl py-5">
              <CardHeader className="px-5">
                <CardTitle asChild>
                  <h2>保存后的状态</h2>
                </CardTitle>
                <CardDescription>新建主题不会立即发起采集。</CardDescription>
              </CardHeader>
              <CardContent className="px-5">
                <dl className="text-muted-foreground mt-4 flex flex-col gap-3 text-sm">
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
                  <Alert variant="destructive" className="mt-5">
                    <AlertTitle>无法保存主题</AlertTitle>
                    <AlertDescription>
                      {submissionError.message}
                      {submissionError.requestId ? (
                        <p className="mt-1 font-mono text-xs opacity-80">
                          请求编号：{submissionError.requestId}
                        </p>
                      ) : null}
                    </AlertDescription>
                  </Alert>
                ) : null}
                <div className="mt-5">
                  <TopicRulePreview
                    matchAny={matchAny}
                    matchAll={matchAll}
                    exclude={exclude}
                    disabled={isSubmitting}
                  />
                </div>
              </CardContent>
              <CardFooter className="bg-transparent px-5 pt-5">
                <Button className="w-full" size="lg" disabled={isSubmitting}>
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
              </CardFooter>
            </Card>
          </aside>
        </form>
      </main>
    </div>
  );
}
