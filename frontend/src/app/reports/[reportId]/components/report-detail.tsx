"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { RotateCcwIcon } from "lucide-react";

import { getReport } from "@/api/ribao";
import { PageState } from "@/components/system/page-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ApiRequestError } from "@/request";

import { ReportMarkdown } from "./report-markdown";

type DetailState =
  | { status: "loading" }
  | { status: "ready"; report: HotKeyAPI.ReportDetailView }
  | { status: "not-found" }
  | { status: "error"; message: string; requestId?: string };

function safeHttpUrl(url: string | null | undefined): string | null {
  if (!url) return null;
  try {
    const parsed = new URL(url);
    return parsed.protocol === "https:" || parsed.protocol === "http:"
      ? parsed.href
      : null;
  } catch {
    return null;
  }
}

function reportTime(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export function ReportDetail({ reportId }: { reportId: string }) {
  const router = useRouter();
  const [state, setState] = useState<DetailState>({ status: "loading" });
  const [retryKey, setRetryKey] = useState(0);

  useEffect(() => {
    let current = true;
    void getReport({ report_id: reportId })
      .then((report) => {
        if (current) setState({ status: "ready", report });
      })
      .catch((error: unknown) => {
        if (!current) return;
        if (
          error instanceof ApiRequestError &&
          error.code === "invalid_session"
        ) {
          router.replace("/login");
        } else if (error instanceof ApiRequestError && error.status === 404) {
          setState({ status: "not-found" });
        } else {
          setState({
            status: "error",
            message:
              error instanceof ApiRequestError
                ? error.message
                : "报告加载失败，请稍后重试。",
            requestId:
              error instanceof ApiRequestError ? error.requestId : undefined,
          });
        }
      });
    return () => {
      current = false;
    };
  }, [reportId, retryKey, router]);

  if (state.status === "loading") {
    return (
      <PageState
        eyebrow="日报"
        title="正在加载"
        description="正在读取报告内容。"
      />
    );
  }
  if (state.status === "not-found") {
    return (
      <PageState
        eyebrow="日报"
        title="报告不存在"
        description="报告已不可访问，或当前账号没有查看权限。"
        action={
          <Button asChild>
            <Link href="/reports">返回报告列表</Link>
          </Button>
        }
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
          <Button
            onClick={() => {
              setState({ status: "loading" });
              setRetryKey((key) => key + 1);
            }}
          >
            <RotateCcwIcon data-icon="inline-start" />
            重试
          </Button>
        }
      />
    );
  }

  const report = state.report;
  return (
    <main className="mx-auto min-h-screen w-full max-w-4xl min-w-0 px-5 py-8 sm:px-8 sm:py-12">
      <Link
        href="/reports"
        className="text-muted-foreground text-sm underline underline-offset-4"
      >
        返回报告列表
      </Link>
      <div className="mt-8 flex flex-wrap items-center gap-2">
        <Badge variant="secondary">
          {report.generator === "model" ? "模型版" : "模板版"}
        </Badge>
        <Badge variant="outline">第 {report.version} 版</Badge>
      </div>
      <h1 className="mt-4 text-3xl font-semibold break-words">
        {report.topic_name}日报
      </h1>
      <dl className="text-muted-foreground mt-4 grid gap-2 text-sm sm:grid-cols-2">
        <div>
          <dt className="inline">窗口：</dt>
          <dd className="inline">
            {reportTime(report.window_start)} 至 {reportTime(report.window_end)}
          </dd>
        </div>
        <div>
          <dt className="inline">截止：</dt>
          <dd className="inline">{reportTime(report.cutoff_at)}</dd>
        </div>
      </dl>
      <ReportMarkdown report={report} />
      {report.citations.length > 0 ? (
        <section aria-labelledby="report-citations" className="mt-12 min-w-0">
          <h2 id="report-citations" className="text-xl font-semibold">
            原帖引用
          </h2>
          <ul className="mt-4 space-y-2">
            {report.citations.map((citation) => {
              const url = safeHttpUrl(citation.url);
              return (
                <li key={citation.citation} className="min-w-0 break-words">
                  <span className="text-muted-foreground mr-2">
                    [{citation.citation}]
                  </span>
                  {url ? (
                    <a
                      href={url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="break-all underline underline-offset-4"
                    >
                      {citation.title}
                    </a>
                  ) : (
                    citation.title
                  )}
                </li>
              );
            })}
          </ul>
        </section>
      ) : null}
    </main>
  );
}
