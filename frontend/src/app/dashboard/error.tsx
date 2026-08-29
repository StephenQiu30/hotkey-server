"use client";

import { AlertCircle } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { PageShell } from "@/layouts/PageShell";

export default function DashboardError({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <PageShell>
      <Alert variant="destructive" className="mx-auto max-w-2xl">
        <AlertCircle />
        <AlertTitle>工作台暂时不可用</AlertTitle>
        <AlertDescription className="space-y-4">
          <p>页面没有完成加载，请稍后重试。若问题持续，请联系工作区管理员。</p>
          <Button type="button" variant="outline" onClick={reset}>
            重试当前页面
          </Button>
        </AlertDescription>
      </Alert>
    </PageShell>
  );
}
