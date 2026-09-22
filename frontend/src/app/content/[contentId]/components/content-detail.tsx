"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ArrowLeftIcon, ExternalLinkIcon, RotateCcwIcon } from "lucide-react";

import { getContentRecord } from "@/api/zuopinziliao";
import {
  contentOriginLabel,
  contentScopeLabel,
  contentScopeNotice,
  formatMetric,
  formatTime,
  hasUnknownMetrics,
  METRIC_LABELS,
  relationTypeLabel,
  truncationReasonLabel,
} from "@/app/content/components/content-presenters";
import { PageState } from "@/components/system/page-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ApiRequestError } from "@/request";

type ContentDetailProps = { contentId: string };

type DetailState =
  | { status: "loading" }
  | { status: "ready"; content: HotKeyAPI.ContentRecordDetailView }
  | { status: "not-found" }
  | { status: "error"; message: string; requestId?: string };

function isInvalidSession(error: unknown): boolean {
  return error instanceof ApiRequestError && error.code === "invalid_session";
}

function DetailItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground text-sm">{label}</dt>
      <dd className="mt-1 font-medium break-words">{value}</dd>
    </div>
  );
}

function ContentVersionSection({
  version,
}: {
  version: HotKeyAPI.ContentVersionView | null;
}) {
  if (version === null) {
    return (
      <section aria-labelledby="content-heading" className="mt-10">
        <h2 id="content-heading" className="text-xl font-medium">
          正文与上下文
        </h2>
        <div className="bg-muted mt-4 rounded-2xl p-6">
          <p className="font-medium">未取得正文</p>
          <p className="text-muted-foreground mt-2 text-sm leading-6">
            当前观察没有可用正文版本；未知不等于空正文。
          </p>
        </div>
      </section>
    );
  }

  const notice = contentScopeNotice(version.text_scope);
  return (
    <section aria-labelledby="content-heading" className="mt-10">
      <h2 id="content-heading" className="text-xl font-medium">
        正文与上下文
      </h2>
      <div className="bg-muted mt-4 rounded-2xl p-6">
        <div className="flex flex-wrap gap-2">
          <Badge variant="secondary">
            {contentScopeLabel(version.text_scope)}
          </Badge>
          <Badge variant="outline">
            {contentOriginLabel(version.text_origin)}
          </Badge>
        </div>
        {notice ? (
          <p className="border-border text-muted-foreground mt-4 border-l-2 pl-4 text-sm leading-6">
            {notice}
          </p>
        ) : null}
        {version.title ? (
          <h3 className="mt-6 text-lg font-medium break-words whitespace-pre-wrap">
            {version.title}
          </h3>
        ) : null}
        {version.body ? (
          <p className="mt-4 leading-7 break-words whitespace-pre-wrap">
            {version.body}
          </p>
        ) : null}
        {version.truncation_reason ? (
          <p className="text-muted-foreground mt-4 text-sm">
            截断原因：{truncationReasonLabel(version.truncation_reason)}
          </p>
        ) : null}
        {version.text_origin_ref ? (
          <p className="text-muted-foreground mt-2 font-mono text-xs break-all">
            提取依据：{version.text_origin_ref}
          </p>
        ) : null}
      </div>

      {version.relations.length > 0 ? (
        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          {version.relations.map((relation) => (
            <article
              key={`${relation.relation_type}:${relation.target_native_scope ?? ""}:${relation.target_external_id}`}
              className="border-border rounded-2xl border p-5"
            >
              <Badge variant="outline">
                {relationTypeLabel(relation.relation_type)}
              </Badge>
              <p className="mt-3 font-mono text-sm break-all">
                {relation.target_external_id}
              </p>
              <p className="text-muted-foreground mt-2 text-xs break-all">
                目标作者：
                {relation.target_author_external_id ?? "未知"}
              </p>
              {relation.target_content_id ? (
                <Link
                  className="mt-3 inline-flex text-sm underline underline-offset-4"
                  href={`/content/${relation.target_content_id}`}
                >
                  查看已获取的目标作品
                </Link>
              ) : (
                <p className="text-muted-foreground mt-3 text-sm">
                  目标作品未获取或当前不可读。
                </p>
              )}
            </article>
          ))}
        </div>
      ) : null}
    </section>
  );
}

export function ContentDetail({ contentId }: ContentDetailProps) {
  const router = useRouter();
  const [state, setState] = useState<DetailState>({ status: "loading" });

  useEffect(() => {
    let isCurrent = true;
    void getContentRecord({ content_id: contentId })
      .then((content) => {
        if (isCurrent) {
          setState({ status: "ready", content });
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
        } else {
          setState(
            error instanceof ApiRequestError
              ? {
                  status: "error",
                  message: error.message,
                  requestId: error.requestId,
                }
              : { status: "error", message: "作品资料加载失败，请稍后重试。" },
          );
        }
      });
    return () => {
      isCurrent = false;
    };
  }, [contentId, router]);

  if (state.status === "loading") {
    return (
      <PageState
        eyebrow="作品资料"
        title="正在读取作品"
        description="正在读取已保存的作品身份、最近观察与发现依据。"
      />
    );
  }

  if (state.status === "not-found") {
    return (
      <PageState
        eyebrow="作品不可用"
        title="没有找到这个作品"
        description="作品不存在、已不可读，或当前使用者无权查看。"
        action={
          <Button asChild variant="secondary">
            <Link href="/content">
              <ArrowLeftIcon data-icon="inline-start" />
              返回作品列表
            </Link>
          </Button>
        }
      />
    );
  }

  if (state.status === "error") {
    return (
      <PageState
        eyebrow="加载失败"
        title="暂时无法读取作品"
        description={
          state.requestId
            ? `${state.message} 请求编号：${state.requestId}`
            : state.message
        }
        action={
          <Button type="button" onClick={() => window.location.reload()}>
            <RotateCcwIcon data-icon="inline-start" />
            重新加载
          </Button>
        }
      />
    );
  }

  const { content } = state;
  const observation = content.latest_observation;

  return (
    <div className="bg-background min-h-screen">
      <header className="mx-auto flex h-16 max-w-5xl items-center px-5 sm:px-8 xl:px-0">
        <Button asChild variant="ghost" size="navigation">
          <Link href="/content">
            <ArrowLeftIcon data-icon="inline-start" />
            返回作品列表
          </Link>
        </Button>
      </header>

      <main className="mx-auto max-w-5xl px-5 py-10 sm:px-8 sm:py-14 xl:px-0">
        <p className="text-muted-foreground font-mono text-xs tracking-wider uppercase">
          Content record
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <h1 className="text-3xl font-semibold tracking-tight break-all sm:text-4xl">
            {content.external_id}
          </h1>
          <Badge variant="outline">{content.source_key}</Badge>
          <Badge
            variant={
              hasUnknownMetrics(observation.metrics) ? "outline" : "secondary"
            }
          >
            {hasUnknownMetrics(observation.metrics)
              ? "部分指标未知"
              : "指标已记录"}
          </Badge>
        </div>
        <p className="text-muted-foreground mt-4 text-sm leading-6">
          当前展示最近一条仍可读取的观察；读取不会刷新来源，也不会用 0
          填补未知值。
        </p>

        <section aria-labelledby="identity-heading" className="mt-10">
          <h2 id="identity-heading" className="text-xl font-medium">
            作品身份
          </h2>
          <dl className="bg-muted mt-4 grid gap-6 rounded-2xl p-6 sm:grid-cols-2">
            <DetailItem label="来源" value={content.source_key} />
            <DetailItem label="对象类型" value={content.object_type} />
            <DetailItem
              label="原生作用域"
              value={content.native_scope ?? "未知"}
            />
            <DetailItem
              label="作者原生 ID"
              value={observation.author_external_id ?? "未知"}
            />
            <DetailItem
              label="发布时间"
              value={formatTime(observation.published_at)}
            />
            <DetailItem
              label="观察时间"
              value={formatTime(observation.observed_at)}
            />
          </dl>
          {observation.canonical_url ? (
            <Button asChild variant="outline" className="mt-4">
              <a
                href={observation.canonical_url}
                target="_blank"
                rel="noreferrer"
              >
                打开原文
                <ExternalLinkIcon data-icon="inline-end" />
              </a>
            </Button>
          ) : (
            <p className="text-muted-foreground mt-4 text-sm">原文链接未知</p>
          )}
        </section>

        <ContentVersionSection version={observation.content_version ?? null} />

        <section aria-labelledby="metrics-heading" className="mt-10">
          <h2 id="metrics-heading" className="text-xl font-medium">
            最近指标观察
          </h2>
          <dl className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3">
            {METRIC_LABELS.map(([key, label]) => (
              <div key={key} className="bg-muted rounded-2xl p-5">
                <dt className="text-muted-foreground text-sm">{label}</dt>
                <dd className="mt-2 text-2xl font-semibold tabular-nums">
                  {formatMetric(observation.metrics[key])}
                </dd>
              </div>
            ))}
          </dl>
        </section>

        <section aria-labelledby="discoveries-heading" className="mt-10">
          <h2 id="discoveries-heading" className="text-xl font-medium">
            发现依据
          </h2>
          <p className="text-muted-foreground mt-2 text-sm">
            同一作品可由多个任务发现；以下仅显示仍有可读观察的任务关系。
          </p>
          <div className="mt-4 grid gap-3">
            {content.discoveries.map((discovery) => (
              <article
                key={discovery.job_id}
                className="bg-muted rounded-2xl p-5"
              >
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0">
                    <h3 className="font-medium break-words">
                      {discovery.configuration_ref}
                    </h3>
                    <p className="text-muted-foreground mt-1 text-xs">
                      配置版本 v{discovery.configuration_version}
                    </p>
                  </div>
                  <Badge variant="outline">
                    {formatTime(discovery.first_observed_at)}
                  </Badge>
                </div>
                <Button asChild variant="ghost" size="sm" className="mt-3">
                  <Link href={`/jobs/${discovery.job_id}`}>查看采集任务</Link>
                </Button>
              </article>
            ))}
          </div>
        </section>
      </main>
    </div>
  );
}
