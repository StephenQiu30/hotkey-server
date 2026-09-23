"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ArrowRightIcon, CircleAlertIcon } from "lucide-react";

import { listContinuousFailureIssues } from "@/api/caijirenwu";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ApiRequestError } from "@/request";

type IssueState =
  | { status: "loading" }
  | { status: "ready"; issues: HotKeyAPI.JobContinuousFailureIssueView[] }
  | { status: "error"; message: string };

function capabilityLabel(capability: HotKeyAPI.SourceCapability): string {
  switch (capability) {
    case "search":
      return "检索";
    case "author_posts":
      return "作者作品";
    case "comments":
      return "评论";
    case "replies":
      return "回复";
    case "page_content":
      return "页面正文";
  }
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "medium",
  }).format(new Date(value));
}

export function JobHealthSummaryView({ state }: { state: IssueState }) {
  if (state.status === "loading") {
    return (
      <p className="text-muted-foreground mt-8 text-sm" role="status">
        正在检查连续失败状态…
      </p>
    );
  }

  if (state.status === "error") {
    return (
      <p className="text-destructive mt-8 text-sm" role="alert">
        连续失败摘要暂不可用，任务历史仍可查看。{state.message}
      </p>
    );
  }

  if (state.issues.length === 0) {
    return (
      <p className="text-muted-foreground mt-8 text-sm" role="status">
        当前没有连续失败提示。
      </p>
    );
  }

  return (
    <section className="mt-8 space-y-3" aria-labelledby="job-issues-title">
      <h2 id="job-issues-title" className="text-lg font-medium">
        需要处理
      </h2>
      {state.issues.map((issue) => (
        <article
          key={issue.latest_failed_job_id}
          className="bg-destructive/10 rounded-2xl p-5 sm:flex sm:items-start sm:justify-between sm:gap-6 sm:p-6"
        >
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="destructive">
                <CircleAlertIcon data-icon="inline-start" />
                连续失败
              </Badge>
              <span className="text-sm font-medium">
                {issue.source_key} · {capabilityLabel(issue.source_capability)}
              </span>
              <span className="text-muted-foreground text-xs">
                配置 {issue.configuration_ref} · v{issue.configuration_version}
              </span>
            </div>
            <p className="mt-3 text-sm">
              最近三条已结束任务均失败。{issue.failure.next_action}
            </p>
            <p className="text-muted-foreground mt-1 text-xs">
              错误代码：{issue.failure.error_code} · 最近失败：
              {formatTime(issue.failure.occurred_at)}
            </p>
          </div>
          <Button
            asChild
            variant="secondary"
            size="sm"
            className="mt-5 sm:mt-0"
          >
            <Link href={`/jobs/${issue.latest_failed_job_id}`}>
              查看最近失败任务
              <ArrowRightIcon data-icon="inline-end" />
            </Link>
          </Button>
        </article>
      ))}
    </section>
  );
}

export function JobHealthSummary() {
  const router = useRouter();
  const [state, setState] = useState<IssueState>({ status: "loading" });

  useEffect(() => {
    let isCurrent = true;
    void listContinuousFailureIssues()
      .then((issues) => {
        if (isCurrent) {
          setState({ status: "ready", issues });
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
        } else {
          setState({
            status: "error",
            message:
              error instanceof ApiRequestError ? error.message : "请稍后重试。",
          });
        }
      });
    return () => {
      isCurrent = false;
    };
  }, [router]);

  return <JobHealthSummaryView state={state} />;
}
