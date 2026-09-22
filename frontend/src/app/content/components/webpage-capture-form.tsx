"use client";

import { useRouter } from "next/navigation";
import { type FormEvent, useRef, useState } from "react";
import { ArrowRightIcon, LoaderCircleIcon } from "lucide-react";

import { createCollectionJob } from "@/api/caijirenwu";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiRequestError } from "@/request";

type SubmissionError = {
  message: string;
  requestId?: string;
};

type PendingOperation = {
  operationId: string;
  target: string;
};

export function validateWebPageTarget(target: string): string | null {
  if (target.length === 0) {
    return "请输入要采集的网页地址。";
  }
  try {
    const url = new URL(target);
    if (url.protocol !== "http:" && url.protocol !== "https:") {
      return "请输入完整的 http:// 或 https:// 地址。";
    }
    return url.username || url.password
      ? "网页地址不能包含用户名或密码。"
      : null;
  } catch {
    return "请输入完整的 http:// 或 https:// 地址。";
  }
}

export function resolvePendingOperation(
  current: PendingOperation | null,
  target: string,
  createId: () => string,
): PendingOperation {
  return current?.target === target
    ? current
    : { operationId: createId(), target };
}

function toSubmissionError(error: unknown): SubmissionError {
  return error instanceof ApiRequestError
    ? { message: error.message, requestId: error.requestId }
    : { message: "网页采集任务提交失败，请稍后重试。" };
}

export function WebPageCaptureForm() {
  const router = useRouter();
  const [target, setTarget] = useState("");
  const [fieldError, setFieldError] = useState<string | null>(null);
  const [submissionError, setSubmissionError] =
    useState<SubmissionError | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const isSubmittingRef = useRef(false);
  const pendingOperationRef = useRef<PendingOperation | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmittingRef.current) {
      return;
    }

    const normalizedTarget = target.trim();
    const validationError = validateWebPageTarget(normalizedTarget);
    if (validationError) {
      setFieldError(validationError);
      return;
    }

    const pendingOperation = resolvePendingOperation(
      pendingOperationRef.current,
      normalizedTarget,
      () => crypto.randomUUID(),
    );
    pendingOperationRef.current = pendingOperation;
    isSubmittingRef.current = true;
    setIsSubmitting(true);
    setFieldError(null);
    setSubmissionError(null);

    try {
      const job = await createCollectionJob({
        operation_id: pendingOperation.operationId,
        kind: "webpage.collect",
        url: normalizedTarget,
      });
      router.push(`/jobs/${job.job_id}`);
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
      isSubmittingRef.current = false;
      setIsSubmitting(false);
    }
  }

  return (
    <section className="bg-muted mt-8 rounded-2xl p-5 sm:p-6">
      <div className="max-w-3xl">
        <h2 className="text-lg font-medium">添加网页</h2>
        <p className="text-muted-foreground mt-2 text-sm leading-6">
          提交当前允许范围内的公开网页。系统会创建后台任务，完成后可从任务页打开已保存资料。
        </p>
      </div>

      <form
        className="mt-5"
        noValidate
        onSubmit={(event) => void handleSubmit(event)}
      >
        <Label htmlFor="webpage-url">网页地址</Label>
        <div className="mt-2 flex flex-col gap-3 sm:flex-row sm:items-start">
          <div className="min-w-0 flex-1">
            <Input
              id="webpage-url"
              name="url"
              type="url"
              inputMode="url"
              autoComplete="url"
              autoCapitalize="none"
              spellCheck={false}
              placeholder="https://example.com/article"
              value={target}
              onChange={(event) => {
                const nextTarget = event.target.value;
                setTarget(nextTarget);
                setFieldError(null);
                setSubmissionError(null);
                if (pendingOperationRef.current?.target !== nextTarget.trim()) {
                  pendingOperationRef.current = null;
                }
              }}
              aria-invalid={fieldError ? true : undefined}
              aria-describedby={fieldError ? "webpage-url-error" : undefined}
              disabled={isSubmitting}
              required
            />
            {fieldError ? (
              <p
                id="webpage-url-error"
                role="alert"
                className="text-destructive mt-2 text-sm"
              >
                {fieldError}
              </p>
            ) : null}
          </div>
          <Button
            type="submit"
            size="lg"
            className="sm:min-w-28"
            disabled={isSubmitting}
          >
            {isSubmitting ? (
              <>
                <LoaderCircleIcon
                  data-icon="inline-start"
                  className="animate-spin"
                />
                正在提交
              </>
            ) : (
              <>
                创建任务
                <ArrowRightIcon data-icon="inline-end" />
              </>
            )}
          </Button>
        </div>
        {submissionError ? (
          <p role="alert" className="text-destructive mt-3 text-sm">
            {submissionError.message}
            {submissionError.requestId
              ? ` 请求编号：${submissionError.requestId}`
              : null}
          </p>
        ) : null}
      </form>
    </section>
  );
}
