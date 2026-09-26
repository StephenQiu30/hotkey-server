"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import {
  ArrowRightIcon,
  CircleAlertIcon,
  LoaderCircleIcon,
} from "lucide-react";

import { initializeOwner } from "@/api/identity";
import { AuthShell } from "@/components/auth/auth-shell";
import {
  CredentialsFields,
  PasswordField,
} from "@/components/auth/credentials-fields";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ApiRequestError } from "@/request";

type SubmissionError = {
  message: string;
  requestId?: string;
  alreadyInitialized?: boolean;
};

function getSubmissionError(error: unknown): SubmissionError {
  if (error instanceof ApiRequestError) {
    if (error.code === "bootstrap_forbidden") {
      return {
        message: "部署密钥无效。请核对服务端配置后重试。",
        requestId: error.requestId,
      };
    }
    if (error.code === "identity_already_initialized") {
      return {
        message: "此部署已完成账户初始化，请直接登录。",
        alreadyInitialized: true,
        requestId: error.requestId,
      };
    }
    if (error.code === "invalid_password") {
      return {
        message: "密码不符合安全要求，请使用至少 12 个字符。",
        requestId: error.requestId,
      };
    }
    return { message: error.message, requestId: error.requestId };
  }
  return { message: "账户初始化失败，请稍后重试。" };
}

export function RegisterForm() {
  const router = useRouter();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [passwordMismatch, setPasswordMismatch] = useState(false);
  const [submissionError, setSubmissionError] =
    useState<SubmissionError | null>(null);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmitting) {
      return;
    }

    const form = event.currentTarget;
    const formData = new FormData(form);
    const password = String(formData.get("password") ?? "");
    if (password !== String(formData.get("confirmPassword") ?? "")) {
      setPasswordMismatch(true);
      setSubmissionError(null);
      return;
    }

    setPasswordMismatch(false);
    setIsSubmitting(true);
    setSubmissionError(null);

    try {
      await initializeOwner(
        {
          username: String(formData.get("username") ?? ""),
          password,
        },
        {
          headers: {
            "Content-Type": "application/json",
            "X-HotKey-Bootstrap-Token": String(
              formData.get("bootstrapToken") ?? "",
            ),
          },
        },
      );
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
      eyebrow="首次部署"
      title="初始化账户"
      description="只有部署者可用配置的密钥创建唯一账户。完成后即可进入工作台。"
      asideTitle="让每条线索，都有来处。"
      asideDescription="先建立你的私有工作台，再用关键词定义你关心的品牌、产品或话题。"
      footer={
        <p className="text-muted-foreground flex flex-wrap items-center gap-1 text-sm">
          已经有账户？
          <Button asChild variant="link" className="h-auto px-0">
            <Link href="/login">返回登录</Link>
          </Button>
        </p>
      }
    >
      <form className="flex flex-col gap-6" onSubmit={handleSubmit}>
        <CredentialsFields
          disabled={isSubmitting}
          passwordAutoComplete="new-password"
        >
          <PasswordField
            id="confirm-password"
            name="confirmPassword"
            label="确认密码"
            autoComplete="new-password"
            disabled={isSubmitting}
            invalid={passwordMismatch}
            error={passwordMismatch ? "两次输入的密码不一致。" : undefined}
            onChange={() => setPasswordMismatch(false)}
          />
          <Field data-disabled={isSubmitting}>
            <FieldLabel htmlFor="bootstrap-token">部署密钥</FieldLabel>
            <Input
              id="bootstrap-token"
              name="bootstrapToken"
              type="password"
              autoComplete="off"
              autoCapitalize="off"
              spellCheck={false}
              minLength={32}
              required
              disabled={isSubmitting}
              className="h-11"
            />
            <FieldDescription>
              仅在首次初始化时使用，由部署者从服务端配置中获取。
            </FieldDescription>
          </Field>
        </CredentialsFields>

        {submissionError ? (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertTitle>无法初始化账户</AlertTitle>
            <AlertDescription>
              {submissionError.message}
              {submissionError.requestId ? (
                <p className="mt-1 font-mono text-xs">
                  请求编号：{submissionError.requestId}
                </p>
              ) : null}
              {submissionError.alreadyInitialized ? (
                <Button asChild variant="link" className="mt-1 h-auto px-0">
                  <Link href="/login">前往登录</Link>
                </Button>
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
          {isSubmitting ? "正在创建" : "创建账户并进入工作台"}
          {!isSubmitting ? <ArrowRightIcon data-icon="inline-end" /> : null}
        </Button>
      </form>
    </AuthShell>
  );
}
