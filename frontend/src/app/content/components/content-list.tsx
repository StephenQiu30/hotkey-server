"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { ArrowRightIcon, ExternalLinkIcon, RotateCcwIcon } from "lucide-react";

import { listContentRecords } from "@/api/zuopinziliao";
import {
  contentScopeLabel,
  contentScopeNotice,
  formatMetric,
  formatTime,
  hasUnknownMetrics,
  safeExternalHref,
  visibilityStatusLabel,
  visibilityStatusNotice,
} from "@/app/content/components/content-presenters";
import { WebPageCaptureForm } from "@/app/content/components/webpage-capture-form";
import { BrandLockup } from "@/components/brand/brand-lockup";
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

type ContentListState =
  | { status: "loading" }
  | {
      status: "ready";
      items: HotKeyAPI.ContentRecordSummaryView[];
      nextCursor: string | null;
    }
  | { status: "error"; message: string; requestId?: string };

function isInvalidSession(error: unknown): boolean {
  return error instanceof ApiRequestError && error.code === "invalid_session";
}

function toErrorState(
  error: unknown,
): Extract<ContentListState, { status: "error" }> {
  return error instanceof ApiRequestError
    ? { status: "error", message: error.message, requestId: error.requestId }
    : { status: "error", message: "作品资料加载失败，请稍后重试。" };
}

function ContentStatus({
  content,
}: {
  content: HotKeyAPI.ContentRecordSummaryView;
}) {
  return hasUnknownMetrics(content.latest_observation.metrics) ? (
    <Badge variant="outline">部分指标未知</Badge>
  ) : (
    <Badge variant="secondary">指标已记录</Badge>
  );
}

function VisibilityStatus({
  visibility,
}: {
  visibility: HotKeyAPI.ContentVisibilityView | null;
}) {
  if (visibility === null) {
    return <Badge variant="outline">来源状态未观察</Badge>;
  }
  const variant =
    visibility.status === "visible"
      ? "secondary"
      : visibility.status === "deleted" || visibility.status === "restricted"
        ? "destructive"
        : "outline";
  return (
    <Badge variant={variant}>{visibilityStatusLabel(visibility.status)}</Badge>
  );
}

function VisibilityNotice({
  visibility,
}: {
  visibility: HotKeyAPI.ContentVisibilityView | null;
}) {
  if (visibility === null || visibility.status === "visible") {
    return null;
  }
  return (
    <p className="text-muted-foreground mt-2 text-xs leading-5">
      {visibilityStatusNotice(visibility.status)}
    </p>
  );
}

function OriginalContentLink({
  canonicalUrl,
}: {
  canonicalUrl: string | null;
}) {
  const href = safeExternalHref(canonicalUrl);
  if (href === null) {
    return null;
  }

  return (
    <a
      className="text-muted-foreground mt-3 inline-flex items-center gap-1 text-xs underline underline-offset-4"
      href={href}
      target="_blank"
      rel="noreferrer"
    >
      打开原文
      <ExternalLinkIcon className="size-3" aria-hidden="true" />
    </a>
  );
}

function PrimaryMetrics({ metrics }: { metrics: HotKeyAPI.ContentMetricView }) {
  return (
    <span className="text-muted-foreground text-xs leading-5">
      点赞 {formatMetric(metrics.like_count)} · 评论{" "}
      {formatMetric(metrics.comment_count)} · 转发{" "}
      {formatMetric(metrics.repost_count)}
    </span>
  );
}

function ContentVersionSummary({
  version,
}: {
  version: HotKeyAPI.ContentVersionView | null;
}) {
  if (version === null) {
    return <p className="text-muted-foreground mt-2 text-xs">未取得正文</p>;
  }
  const preview = version.title ?? version.body;
  const notice = contentScopeNotice(version.text_scope);
  return (
    <div className="mt-2 min-w-0">
      <Badge variant="secondary">{contentScopeLabel(version.text_scope)}</Badge>
      {preview ? (
        <p className="mt-2 line-clamp-2 text-sm break-words whitespace-pre-wrap">
          {preview}
        </p>
      ) : null}
      {notice ? (
        <p className="text-muted-foreground mt-1 line-clamp-2 text-xs">
          {notice}
        </p>
      ) : null}
    </div>
  );
}

function LoadingContentList() {
  return (
    <div aria-label="正在读取作品资料" className="mt-8 space-y-3">
      {[0, 1, 2].map((item) => (
        <Skeleton key={item} className="h-24 w-full rounded-2xl" />
      ))}
    </div>
  );
}

export function ContentList() {
  const router = useRouter();
  const [state, setState] = useState<ContentListState>({ status: "loading" });
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setState({ status: "loading" });
    try {
      const page = await listContentRecords({ limit: 20 });
      setState({
        status: "ready",
        items: page.items,
        nextCursor: page.next_cursor,
      });
    } catch (error) {
      if (isInvalidSession(error)) {
        router.replace("/login");
        return;
      }
      setState(toErrorState(error));
    }
  }, [router]);

  useEffect(() => {
    let isCurrent = true;
    void listContentRecords({ limit: 20 })
      .then((page) => {
        if (isCurrent) {
          setState({
            status: "ready",
            items: page.items,
            nextCursor: page.next_cursor,
          });
        }
      })
      .catch((error: unknown) => {
        if (!isCurrent) {
          return;
        }
        if (isInvalidSession(error)) {
          router.replace("/login");
        } else {
          setState(toErrorState(error));
        }
      });
    return () => {
      isCurrent = false;
    };
  }, [router]);

  async function loadMore() {
    if (
      state.status !== "ready" ||
      state.nextCursor === null ||
      isLoadingMore
    ) {
      return;
    }
    setIsLoadingMore(true);
    setLoadMoreError(null);
    try {
      const page = await listContentRecords({
        cursor: state.nextCursor,
        limit: 20,
      });
      setState({
        status: "ready",
        items: [...state.items, ...page.items],
        nextCursor: page.next_cursor,
      });
    } catch (error) {
      if (isInvalidSession(error)) {
        router.replace("/login");
      } else {
        setLoadMoreError(
          error instanceof ApiRequestError
            ? error.message
            : "后续作品加载失败，请重试。",
        );
      }
    } finally {
      setIsLoadingMore(false);
    }
  }

  return (
    <div className="bg-background min-h-screen">
      <header className="mx-auto flex h-16 max-w-7xl items-center justify-between px-5 sm:px-8 xl:px-16 2xl:px-0">
        <BrandLockup href="/events" />
        <Button asChild variant="ghost" size="navigation">
          <Link href="/events">返回工作台</Link>
        </Button>
      </header>

      <main className="mx-auto max-w-7xl px-5 py-12 sm:px-8 sm:py-16 xl:px-16 2xl:px-0">
        <p className="text-muted-foreground font-mono text-xs tracking-wider uppercase">
          Content
        </p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight sm:text-4xl">
          作品资料
        </h1>
        <p className="text-muted-foreground mt-4 max-w-2xl leading-7">
          查看已持久保存且仍可读的作品身份、正文边界与最近观察。摘要、截断和未知保持原语义，读取不会触发来源请求。
        </p>

        <WebPageCaptureForm />

        {state.status === "error" ? (
          <section className="bg-destructive/10 mt-8 rounded-2xl p-6 sm:p-8">
            <h2 className="text-lg font-medium">暂时无法读取作品</h2>
            <p className="text-muted-foreground mt-2 text-sm leading-6">
              {state.message}
              {state.requestId ? ` 请求编号：${state.requestId}` : null}
            </p>
            <Button className="mt-5" onClick={() => void load()}>
              <RotateCcwIcon data-icon="inline-start" />
              重新加载
            </Button>
          </section>
        ) : null}

        {state.status === "loading" ? <LoadingContentList /> : null}

        {state.status === "ready" && state.items.length === 0 ? (
          <section className="bg-muted mt-8 rounded-2xl px-6 py-14 text-center sm:px-10">
            <h2 className="text-lg font-medium">尚无可读作品</h2>
            <p className="text-muted-foreground mx-auto mt-2 max-w-md text-sm leading-6">
              完成受控采集并持久保存后，作品会显示在这里；当前为空不代表来源返回了零结果。
            </p>
          </section>
        ) : null}

        {state.status === "ready" && state.items.length > 0 ? (
          <>
            <div className="mt-8 hidden md:block">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>作品身份</TableHead>
                    <TableHead>最近观察</TableHead>
                    <TableHead>指标</TableHead>
                    <TableHead>发现依据</TableHead>
                    <TableHead className="text-right">查看</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {state.items.map((content) => (
                    <TableRow key={content.id}>
                      <TableCell className="max-w-xs whitespace-normal">
                        <div className="flex flex-wrap items-center gap-2">
                          <Badge variant="outline">{content.source_key}</Badge>
                          <ContentStatus content={content} />
                          <VisibilityStatus
                            visibility={content.current_visibility}
                          />
                        </div>
                        <p className="mt-2 font-mono text-xs break-all">
                          {content.external_id}
                        </p>
                        <ContentVersionSummary
                          version={
                            content.latest_observation.content_version ?? null
                          }
                        />
                        <VisibilityNotice
                          visibility={content.current_visibility}
                        />
                        <p className="text-muted-foreground mt-1 text-xs">
                          作者：
                          {content.latest_observation.author_external_id ??
                            "未知"}
                        </p>
                      </TableCell>
                      <TableCell className="whitespace-normal">
                        <p>
                          {formatTime(content.latest_observation.observed_at)}
                        </p>
                        <p className="text-muted-foreground mt-1 text-xs">
                          发布：
                          {formatTime(content.latest_observation.published_at)}
                        </p>
                      </TableCell>
                      <TableCell className="max-w-xs whitespace-normal">
                        <PrimaryMetrics
                          metrics={content.latest_observation.metrics}
                        />
                      </TableCell>
                      <TableCell>{content.discovery_count}</TableCell>
                      <TableCell className="text-right">
                        <Button asChild variant="ghost" size="sm">
                          <Link href={`/content/${content.id}`}>
                            详情
                            <ArrowRightIcon data-icon="inline-end" />
                          </Link>
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            <div className="mt-8 grid gap-3 md:hidden">
              {state.items.map((content) => (
                <article key={content.id} className="bg-muted rounded-2xl p-5">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline">{content.source_key}</Badge>
                    <ContentStatus content={content} />
                    <VisibilityStatus visibility={content.current_visibility} />
                  </div>
                  <h2 className="mt-4 font-mono text-sm font-medium break-all">
                    {content.external_id}
                  </h2>
                  <ContentVersionSummary
                    version={content.latest_observation.content_version ?? null}
                  />
                  <VisibilityNotice visibility={content.current_visibility} />
                  <p className="text-muted-foreground mt-2 text-xs">
                    作者：
                    {content.latest_observation.author_external_id ?? "未知"}
                  </p>
                  <p className="text-muted-foreground mt-1 text-xs">
                    观察于 {formatTime(content.latest_observation.observed_at)}
                  </p>
                  <p className="mt-3">
                    <PrimaryMetrics
                      metrics={content.latest_observation.metrics}
                    />
                  </p>
                  <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
                    <span className="text-muted-foreground text-xs">
                      {content.discovery_count} 条发现依据
                    </span>
                    <Button asChild variant="secondary" size="sm">
                      <Link href={`/content/${content.id}`}>
                        查看详情
                        <ArrowRightIcon data-icon="inline-end" />
                      </Link>
                    </Button>
                  </div>
                  <OriginalContentLink
                    canonicalUrl={content.latest_observation.canonical_url}
                  />
                </article>
              ))}
            </div>

            {state.nextCursor ? (
              <div className="mt-8 flex justify-center">
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => void loadMore()}
                  disabled={isLoadingMore}
                >
                  {isLoadingMore ? "正在加载" : "加载更多"}
                </Button>
              </div>
            ) : null}
            {loadMoreError ? (
              <p
                role="alert"
                className="text-destructive mt-3 text-center text-sm"
              >
                {loadMoreError}
              </p>
            ) : null}
          </>
        ) : null}
      </main>
    </div>
  );
}
