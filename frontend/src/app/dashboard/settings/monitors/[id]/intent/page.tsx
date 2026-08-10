"use client";

import Link from "next/link";
import { ArrowLeft, BrainCircuit, SearchCheck } from "lucide-react";
import { useParams } from "next/navigation";
import { MonitorIntentWorkspace } from "@/components/dashboard/MonitorIntentWorkspace";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

export default function MonitorIntentPage() {
  const params = useParams<{ id: string }>();
  const user = useAuthStore((state) => state.user);
  const monitorID = Number(params.id);
  const validMonitorID = Number.isSafeInteger(monitorID) && monitorID > 0;
  const canEdit = user?.role === UserRole.Editor || user?.role === UserRole.Admin;
  const canAdmin = user?.role === UserRole.Admin;

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
            {validMonitorID ? (
              <Button asChild variant="outline">
                <Link href={`/dashboard/settings/monitors/${monitorID}/matches`}>
                  <SearchCheck />
                  查看匹配判定
                </Link>
              </Button>
            ) : null}
          </div>
        }
        description="把自然语言目标、硬条件、实体和正反例保存为独立、可审计的草稿版本。"
        eyebrow="Semantic Monitoring"
        title="语义监控意图"
      />
      {!validMonitorID ? (
        <Card className="mt-6 items-center p-10 text-center" role="alert">
          <BrainCircuit className="text-muted-foreground" />
          <p className="font-medium">监控编号无效</p>
          <p className="text-sm text-muted-foreground">请从热点监控列表重新进入。</p>
        </Card>
      ) : !canEdit ? (
        <Card className="mt-6 items-center p-10 text-center" role="alert">
          <BrainCircuit className="text-muted-foreground" />
          <p className="font-medium">当前角色不能编辑语义意图</p>
          <p className="text-sm text-muted-foreground">
            查看者只能读取已发布监控；请联系编辑者或管理员。
          </p>
        </Card>
      ) : (
        <div className="mt-6">
          <MonitorIntentWorkspace canAdmin={canAdmin} monitorID={monitorID} />
        </div>
      )}
    </div>
  );
}
