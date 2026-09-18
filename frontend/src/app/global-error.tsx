"use client";

import { RotateCcwIcon } from "lucide-react";

import { PageState } from "@/components/patterns/page-state";
import { Button } from "@/components/ui/button";

type GlobalErrorProps = {
  error: Error & { digest?: string };
  reset: () => void;
};

export default function GlobalError({ reset }: GlobalErrorProps) {
  return (
    <html lang="zh-CN">
      <body>
        <PageState
          eyebrow="应用恢复"
          title="HotKey 暂时无法显示。"
          description="请重新加载应用。错误详情不会显示在公开页面中。"
          action={
            <Button type="button" onClick={reset} size="lg">
              重新加载
              <RotateCcwIcon data-icon="inline-end" />
            </Button>
          }
        />
      </body>
    </html>
  );
}
