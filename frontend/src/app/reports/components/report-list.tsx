"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { ArrowRightIcon, RotateCcwIcon } from "lucide-react";

import { listMonitorTopics } from "@/api/jiankongzhuti";
import { listReports } from "@/api/ribao";
import { PageState } from "@/components/system/page-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ApiRequestError } from "@/request";

type ListState =
  | { status: "loading" }
  | {
      status: "ready";
      items: HotKeyAPI.ReportSummaryView[];
      topics: HotKeyAPI.MonitorTopicView[];
      nextCursor: string | null;
    }
  | { status: "error"; message: string; requestId?: string };

function reportDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "long",
    day: "numeric",
  }).format(new Date(value));
}

async function fetchReportPage(
  topicId: string,
  dateFrom: string,
  dateTo: string,
): Promise<{
  topics: HotKeyAPI.MonitorTopicView[];
  page: Awaited<ReturnType<typeof listReports>>;
}> {
  const topics: HotKeyAPI.MonitorTopicView[] = [];
  let topicCursor: string | null = null;
  do {
    const topicPage: HotKeyAPI.PageViewMonitorTopicView_ =
      await listMonitorTopics({
        limit: 50,
        include_archived: true,
        cursor: topicCursor ?? undefined,
      });
    topics.push(...topicPage.items);
    topicCursor = topicPage.next_cursor;
  } while (topicCursor);
  const page = await listReports({
    kind: "daily",
    topic_id: topicId === "all" ? undefined : topicId,
    date_from: dateFrom || undefined,
    date_to: dateTo || undefined,
    limit: 20,
  });
  return { topics, page };
}

export function ReportList() {
  const router = useRouter();
  const [state, setState] = useState<ListState>({ status: "loading" });
  const [topicId, setTopicId] = useState("all");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadedFilterKey, setLoadedFilterKey] = useState<string | null>(null);
  const [pageError, setPageError] = useState<string | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const requestNumber = useRef(0);
  const filterKey = `${topicId}|${dateFrom}|${dateTo}`;
  const rangeError =
    dateFrom && dateTo && dateFrom > dateTo ? "开始日期不能晚于结束日期" : null;
  const filterLoading =
    rangeError === null &&
    state.status === "ready" &&
    loadedFilterKey !== filterKey;

  useEffect(() => {
    const currentRequest = ++requestNumber.current;
    if (rangeError !== null) return;
    let isCurrent = true;
    void fetchReportPage(topicId, dateFrom, dateTo)
      .then(({ topics, page }) => {
        if (!isCurrent || currentRequest !== requestNumber.current) return;
        setPageError(null);
        setLoadedFilterKey(filterKey);
        setState({
          status: "ready",
          topics,
          items: page.items,
          nextCursor: page.next_cursor,
        });
      })
      .catch((error: unknown) => {
        if (!isCurrent || currentRequest !== requestNumber.current) return;
        if (
          error instanceof ApiRequestError &&
          error.code === "invalid_session"
        ) {
          router.replace("/login");
          return;
        }
        setLoadedFilterKey(filterKey);
        setState({
          status: "error",
          message:
            error instanceof ApiRequestError
              ? error.message
              : "日报加载失败，请稍后重试。",
          requestId:
            error instanceof ApiRequestError ? error.requestId : undefined,
        });
      });
    return () => {
      isCurrent = false;
    };
  }, [router, topicId, dateFrom, dateTo, filterKey, rangeError, reloadToken]);

  function reload() {
    setState({ status: "loading" });
    setReloadToken((value) => value + 1);
  }

  async function loadMore() {
    if (
      state.status !== "ready" ||
      !state.nextCursor ||
      loadingMore ||
      filterLoading
    )
      return;
    const currentRequest = requestNumber.current;
    setLoadingMore(true);
    setPageError(null);
    try {
      const page = await listReports({
        kind: "daily",
        topic_id: topicId === "all" ? undefined : topicId,
        date_from: dateFrom || undefined,
        date_to: dateTo || undefined,
        cursor: state.nextCursor,
        limit: 20,
      });
      if (currentRequest !== requestNumber.current) return;
      setState({
        ...state,
        items: [...state.items, ...page.items],
        nextCursor: page.next_cursor,
      });
    } catch (error) {
      if (currentRequest !== requestNumber.current) return;
      setPageError(
        error instanceof ApiRequestError
          ? error.message
          : "更多日报加载失败，请重试。",
      );
    } finally {
      setLoadingMore(false);
    }
  }

  if (state.status === "loading") {
    return (
      <PageState
        eyebrow="日报"
        title="正在加载"
        description="正在读取你的报告。"
      />
    );
  }
  if (state.status === "error") {
    return (
      <PageState
        eyebrow="日报"
        title="暂时无法读取报告"
        description={
          state.requestId
            ? `${state.message} 请求编号：${state.requestId}`
            : state.message
        }
        action={
          <Button onClick={reload}>
            <RotateCcwIcon data-icon="inline-start" />
            重试
          </Button>
        }
      />
    );
  }

  return (
    <main className="mx-auto min-h-screen w-full max-w-5xl min-w-0 px-5 py-8 sm:px-8 sm:py-12">
      <Link
        href="/events"
        className="text-muted-foreground text-sm underline underline-offset-4"
      >
        返回工作台
      </Link>
      <div className="mt-8 flex flex-wrap items-end justify-between gap-4">
        <div>
          <Badge variant="secondary">日报</Badge>
          <h1 className="mt-3 text-3xl font-semibold">报告</h1>
          <p className="text-muted-foreground mt-2 text-sm">
            按主题和日期查看已定稿的报告。
          </p>
        </div>
        <Button variant="outline" onClick={reload}>
          <RotateCcwIcon data-icon="inline-start" />
          刷新
        </Button>
      </div>
      <section
        aria-label="报告筛选"
        className="mt-8 grid min-w-0 gap-4 sm:grid-cols-3"
      >
        <div className="min-w-0 space-y-2">
          <Label htmlFor="report-topic">主题</Label>
          <Select value={topicId} onValueChange={setTopicId}>
            <SelectTrigger id="report-topic" className="w-full min-w-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部主题</SelectItem>
              {state.topics.map((topic) => (
                <SelectItem key={topic.id} value={topic.id}>
                  {topic.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="min-w-0 space-y-2">
          <Label htmlFor="report-date-from">开始日期</Label>
          <Input
            id="report-date-from"
            type="date"
            className="w-full min-w-0"
            value={dateFrom}
            onChange={(event) => setDateFrom(event.target.value)}
          />
        </div>
        <div className="min-w-0 space-y-2">
          <Label htmlFor="report-date-to">结束日期</Label>
          <Input
            id="report-date-to"
            type="date"
            className="w-full min-w-0"
            value={dateTo}
            onChange={(event) => setDateTo(event.target.value)}
          />
        </div>
      </section>
      {(rangeError ?? pageError) ? (
        <p role="alert" className="text-destructive mt-4 text-sm">
          {rangeError ?? pageError}
        </p>
      ) : null}
      {filterLoading ? (
        <p role="status" className="text-muted-foreground mt-4 text-sm">
          正在筛选报告…
        </p>
      ) : null}
      <section aria-label="报告列表" className="mt-8 space-y-3">
        {state.items.length === 0 ? (
          <div className="bg-muted rounded-2xl p-6 text-sm">
            当前条件下暂无已定稿报告。
          </div>
        ) : (
          state.items.map((report) => (
            <article
              key={report.id}
              className="bg-muted flex min-w-0 flex-wrap items-center justify-between gap-4 rounded-2xl p-5 sm:p-6"
            >
              <div className="min-w-0">
                <p className="font-medium break-words">
                  {reportDate(report.window_start)} · {report.topic_name}
                </p>
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  <Badge variant="secondary">
                    {report.generator === "model" ? "模型版" : "模板版"}
                  </Badge>
                  <span className="text-muted-foreground text-xs">
                    第 {report.version} 版
                  </span>
                </div>
              </div>
              <Button asChild variant="outline">
                <Link href={`/reports/${report.id}`}>
                  查看报告
                  <ArrowRightIcon data-icon="inline-end" />
                </Link>
              </Button>
            </article>
          ))
        )}
      </section>
      {state.nextCursor ? (
        <Button
          className="mt-6"
          variant="outline"
          disabled={loadingMore || filterLoading}
          onClick={() => void loadMore()}
        >
          {loadingMore ? "正在加载" : "加载更多"}
        </Button>
      ) : null}
    </main>
  );
}
