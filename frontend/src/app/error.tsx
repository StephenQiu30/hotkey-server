"use client";

import { RotateCcwIcon } from "lucide-react";

import { PageState } from "@/components/patterns/page-state";
import { Button } from "@/components/ui/button";

type ErrorBoundaryProps = {
  error: Error & { digest?: string };
  reset: () => void;
};

export default function ErrorBoundary({ reset }: ErrorBoundaryProps) {
  return (
    <PageState
      eyebrow="暂时不可用"
      title="页面没有正常完成加载。"
      description="当前操作没有丢失。请重新尝试；如果问题持续存在，再检查服务状态。"
      action={
        <Button type="button" onClick={reset} size="lg">
          重新尝试
          <RotateCcwIcon data-icon="inline-end" />
        </Button>
      }
    />
  );
}
