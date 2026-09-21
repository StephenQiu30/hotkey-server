"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import { ArrowLeftIcon, ArrowRightIcon, LoaderCircleIcon } from "lucide-react";

import { createIdentitySession } from "@/api/identity";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiRequestError } from "@/request";

type SubmissionError = {
  message: string;
  requestId?: string;
};

function getSubmissionError(error: unknown): SubmissionError {
  if (error instanceof ApiRequestError) {
    if (error.code === "invalid_credentials") {
      return { message: "用户名或密码错误，请重新输入。" };
    }
    return {
      message: error.message,
      requestId: error.requestId,
    };
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
    <main className="bg-background flex min-h-screen items-center px-5 py-12 sm:px-8">
      <section className="mx-auto w-full max-w-sm">
        <Button asChild variant="ghost" className="-ml-2.5">
          <Link href="/">
            <ArrowLeftIcon data-icon="inline-start" />
            返回首页
          </Link>
        </Button>

        <div className="bg-muted mt-8 rounded-2xl p-6 sm:p-8">
          <div className="bg-primary text-primary-foreground flex size-9 items-center justify-center rounded-lg font-mono text-xs">
            HK
          </div>
          <h1 className="mt-6 text-3xl font-semibold tracking-tight">
            登录工作台
          </h1>
          <p className="text-muted-foreground mt-2 text-sm leading-6">
            使用部署时初始化的 owner 凭据继续。
          </p>

          <form className="mt-8 space-y-5" onSubmit={handleSubmit}>
            <div className="space-y-2">
              <Label htmlFor="username">用户名</Label>
              <Input
                id="username"
                name="username"
                autoComplete="username"
                minLength={3}
                maxLength={64}
                required
                disabled={isSubmitting}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">密码</Label>
              <Input
                id="password"
                name="password"
                type="password"
                autoComplete="current-password"
                minLength={12}
                maxLength={128}
                required
                disabled={isSubmitting}
              />
            </div>

            {submissionError ? (
              <div
                role="alert"
                className="text-destructive bg-destructive/10 rounded-lg px-3 py-2.5 text-sm leading-5"
              >
                <p>{submissionError.message}</p>
                {submissionError.requestId ? (
                  <p className="mt-1 font-mono text-xs opacity-80">
                    请求编号：{submissionError.requestId}
                  </p>
                ) : null}
              </div>
            ) : null}

            <Button className="w-full" size="lg" disabled={isSubmitting}>
              {isSubmitting ? (
                <LoaderCircleIcon className="animate-spin" aria-hidden="true" />
              ) : (
                <ArrowRightIcon data-icon="inline-end" />
              )}
              {isSubmitting ? "正在登录" : "进入工作台"}
            </Button>
          </form>
        </div>
      </section>
    </main>
  );
}
