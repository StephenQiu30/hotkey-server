"use client";

import Link from "next/link";
import { ArrowLeft, BrainCircuit, SearchCheck } from "lucide-react";
import { useParams } from "next/navigation";
import { DocumentMatchWorkspace } from "@/components/dashboard/DocumentMatchWorkspace";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

export default function DocumentMatchesPage() {
  const params = useParams<{ id: string }>();
  const user = useAuthStore((state) => state.user);
  const monitorID = Number(params.id);
  const validMonitorID = Number.isSafeInteger(monitorID) && monitorID > 0;
  const canReview = user?.role === UserRole.Editor || user?.role === UserRole.Admin;

  return (
    <div className="app-page">
      <PageHeader
        action={
          <div className="flex flex-col gap-2 sm:flex-row">
            <Button asChild variant="outline">
              <Link href="/dashboard/settings">
                <ArrowLeft />
                返回热点监控
              </Link>
            </Button>
            {validMonitorID && canReview ? (
              <Button asChild variant="outline">
                <Link href={`/dashboard/settings/monitors/${monitorID}/intent`}>
                  <BrainCircuit />
                  编辑语义意图
                </Link>
              </Button>
            ) : null}
          </div>
        }
        description="查看精确监控版本与正文版本的召回信号，并追加人工相关或不相关判定；这里不判断报道真假。"
        eyebrow="Semantic Monitoring"
        title="相关性判定"
      />
      {!validMonitorID ? (
        <Card className="mt-6 items-center p-10 text-center" role="alert">
          <SearchCheck className="text-muted-foreground" />
          <p className="font-medium">监控编号无效</p>
          <p className="text-sm text-muted-foreground">请从热点监控列表重新进入。</p>
        </Card>
      ) : (
        <div className="mt-6">
          <DocumentMatchWorkspace canReview={canReview} monitorID={monitorID} />
        </div>
      )}
    </div>
  );
}
