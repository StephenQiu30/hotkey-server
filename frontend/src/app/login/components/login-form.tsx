"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import {
  ArrowRightIcon,
  CircleAlertIcon,
  LoaderCircleIcon,
} from "lucide-react";

import { createIdentitySession } from "@/api/identity";
import { AuthShell } from "@/components/auth/auth-shell";
import { CredentialsFields } from "@/components/auth/credentials-fields";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { ApiRequestError } from "@/request";

type SubmissionError = {
  message: string;
  requestId?: string;
};

function getSubmissionError(error: unknown): SubmissionError {
  if (error instanceof ApiRequestError) {
    if (error.code === "invalid_credentials") {
      return {
        message: "用户名或密码错误，请重新输入。",
        requestId: error.requestId,
      };
    }
    return { message: error.message, requestId: error.requestId };
  }
  return { message: "登录失败，请稍后重试。" };
}

export function LoginForm() {
  const router = useRouter();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submissionError, setSubmissionError] =
    useState<SubmissionError | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmitting) {
      return;
    }

    const form = event.currentTarget;
    const formData = new FormData(form);
    setIsSubmitting(true);
    setSubmissionError(null);

    try {
      await createIdentitySession({
        username: String(formData.get("username") ?? ""),
        password: String(formData.get("password") ?? ""),
      });
      form.reset();
      router.replace("/events");
      router.refresh();
    } catch (error) {
      setSubmissionError(getSubmissionError(error));
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <AuthShell
      eyebrow="登录工作台"
      title="欢迎回来"
      description="登录后继续查看你定义的主题与已汇集的线索。"
      asideTitle="从一个关键词，看见正在发生的变化。"
      asideDescription="让分散的讨论沿来源与时间汇聚，回到真正值得关注的主题。"
      footer={
        <p className="text-muted-foreground flex flex-wrap items-center gap-1 text-sm">
          首次部署？
          <Button asChild variant="link" className="h-auto px-0">
            <Link href="/register">初始化账户</Link>
          </Button>
        </p>
      }
    >
      <form className="flex flex-col gap-6" onSubmit={handleSubmit}>
        <CredentialsFields
          disabled={isSubmitting}
          passwordAutoComplete="current-password"
        />

        {submissionError ? (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertTitle>无法登录</AlertTitle>
            <AlertDescription>
              {submissionError.message}
              {submissionError.requestId ? (
                <p className="mt-1 font-mono text-xs">
                  请求编号：{submissionError.requestId}
                </p>
              ) : null}
            </AlertDescription>
          </Alert>
        ) : null}

        <Button className="h-11 w-full" size="lg" disabled={isSubmitting}>
          {isSubmitting ? (
            <LoaderCircleIcon
              data-icon="inline-start"
              className="animate-spin"
            />
          ) : null}
          {isSubmitting ? "正在登录" : "进入工作台"}
          {!isSubmitting ? <ArrowRightIcon data-icon="inline-end" /> : null}
        </Button>
      </form>
    </AuthShell>
  );
}
