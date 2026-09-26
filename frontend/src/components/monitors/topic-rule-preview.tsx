"use client";

import { type FormEvent, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { LoaderCircleIcon, SearchCheckIcon } from "lucide-react";

import { previewMonitorTopic } from "@/api/jiankongzhuti";
import { parseKeywordLines } from "@/components/monitors/keyword-group-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ApiRequestError } from "@/request";

type TopicRulePreviewProps = {
  matchAny: string;
  matchAll: string;
  exclude: string;
  disabled?: boolean;
};

type PreviewState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "ready"; preview: HotKeyAPI.MonitorTopicPreviewView }
  | { status: "error"; message: string; requestId?: string };

export function TopicRulePreview({
  matchAny,
  matchAll,
  exclude,
  disabled = false,
}: TopicRulePreviewProps) {
  const router = useRouter();
  const [sampleTitle, setSampleTitle] = useState("");
  const [state, setState] = useState<PreviewState>({ status: "idle" });
  const submittingRef = useRef(false);

  async function handlePreview(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    event.stopPropagation();
    if (submittingRef.current) return;
    const any = parseKeywordLines(matchAny);
    const all = parseKeywordLines(matchAll);
    if (any.length === 0 && all.length === 0) {
      setState({
        status: "error",
        message: "至少填写一个“任意命中”或“全部包含”关键词。",
      });
      return;
    }

    submittingRef.current = true;
    setState({ status: "loading" });
    try {
      const preview = await previewMonitorTopic({
        match_any: any,
        match_all: all,
        exclude: parseKeywordLines(exclude),
        sample_titles: [sampleTitle],
      });
      setState({ status: "ready", preview });
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
            ? error.status === 422 && error.details?.[0]
              ? error.details[0].message
              : error.message
            : "规则预览失败，请稍后重试。",
        requestId:
          error instanceof ApiRequestError ? error.requestId : undefined,
      });
    } finally {
      submittingRef.current = false;
    }
  }

  const sample = state.status === "ready" ? state.preview.samples[0] : null;
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="secondary"
          className="w-full"
          disabled={disabled}
        >
          <SearchCheckIcon data-icon="inline-start" />
          预览规则
        </Button>
      </DialogTrigger>
      <DialogContent
        className="max-h-[calc(100dvh-2rem)] max-w-lg overflow-y-auto"
        showCloseButton={false}
      >
        <DialogHeader>
          <DialogTitle>本地规则预览</DialogTitle>
          <DialogDescription>
            输入一条标题检查组合逻辑。预览不会保存主题、创建任务或访问外部来源。
          </DialogDescription>
        </DialogHeader>

        <form className="space-y-4" onSubmit={handlePreview}>
          <div className="space-y-2">
            <Label htmlFor="preview-sample-title">标题样本</Label>
            <Textarea
              id="preview-sample-title"
              value={sampleTitle}
              onChange={(event) => setSampleTitle(event.target.value)}
              maxLength={500}
              required
              placeholder="例如：品牌召回招聘公告"
            />
          </div>
          <Button type="submit" disabled={state.status === "loading"}>
            {state.status === "loading" ? (
              <LoaderCircleIcon className="animate-spin" aria-hidden="true" />
            ) : (
              <SearchCheckIcon data-icon="inline-start" />
            )}
            {state.status === "loading" ? "正在检查" : "检查标题"}
          </Button>
        </form>

        {state.status === "error" ? (
          <div
            className="bg-destructive/10 rounded-lg p-3 text-sm"
            role="alert"
          >
            <p>{state.message}</p>
            {state.requestId ? (
              <p className="mt-1 font-mono text-xs">
                请求编号：{state.requestId}
              </p>
            ) : null}
          </div>
        ) : null}

        {state.status === "ready" && sample ? (
          <div className="space-y-4" aria-live="polite">
            <div className="bg-muted rounded-xl p-4">
              <div className="flex items-center gap-2">
                <Badge variant={sample.matched ? "secondary" : "outline"}>
                  {sample.matched
                    ? "命中"
                    : sample.excluded_by.length > 0
                      ? "已排除"
                      : "未命中"}
                </Badge>
                {sample.excluded_by.length > 0 ? (
                  <span className="text-muted-foreground text-xs">
                    排除原因：{sample.excluded_by.join("、")}
                  </span>
                ) : null}
              </div>
              <dl className="text-muted-foreground mt-4 space-y-2 text-xs leading-5">
                <div>
                  <dt className="text-foreground font-medium">本次任意命中</dt>
                  <dd>{sample.matched_any.join("、") || "无"}</dd>
                </div>
                <div>
                  <dt className="text-foreground font-medium">
                    本次全部包含命中
                  </dt>
                  <dd>
                    {state.preview.rules.match_all.length === 0
                      ? "未配置（不限制）"
                      : `${sample.matched_all.join("、") || "无"}；命中 ${sample.matched_all.length}/${state.preview.rules.match_all.length} 项`}
                  </dd>
                </div>
              </dl>
              <dl className="text-muted-foreground mt-4 space-y-2 text-xs leading-5">
                <div>
                  <dt className="text-foreground font-medium">任意命中</dt>
                  <dd>
                    {state.preview.rules.match_any.join("、") || "不限制"}
                  </dd>
                </div>
                <div>
                  <dt className="text-foreground font-medium">全部包含</dt>
                  <dd>
                    {state.preview.rules.match_all.join("、") || "不限制"}
                  </dd>
                </div>
                <div>
                  <dt className="text-foreground font-medium">排除优先</dt>
                  <dd>{state.preview.rules.exclude.join("、") || "无"}</dd>
                </div>
              </dl>
            </div>

            <div className="ring-foreground/10 rounded-xl p-4 text-sm ring-1">
              <p className="font-medium">查询与预算影响</p>
              <p className="text-muted-foreground mt-2 leading-6">
                本地别名匹配：
                {state.preview.expansion.local_alias_external_queries}
                次外部查询，
                {state.preview.expansion.local_alias_budget_units}
                预算单位。
              </p>
              <p className="text-muted-foreground mt-1 leading-6">
                上游扩词：
                {state.preview.expansion.upstream_status ===
                "pending_source_selection"
                  ? "来源尚未选择"
                  : "状态未知"}
                ，查询次数
                {state.preview.expansion.upstream_external_queries ?? "未知"}
                ，预算
                {state.preview.expansion.upstream_budget_units ?? "未知"}
                ，当前不能启用。
              </p>
            </div>
          </div>
        ) : null}
        <DialogClose asChild>
          <Button type="button" variant="ghost" className="justify-self-end">
            关闭预览
          </Button>
        </DialogClose>
      </DialogContent>
    </Dialog>
  );
}
