"use client";

import Link from "next/link";
import { ArrowLeft, FileText } from "lucide-react";
import { useParams } from "next/navigation";
import { DocumentVersionWorkspace } from "@/components/dashboard/DocumentVersionWorkspace";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { PageShell } from "@/layouts/PageShell";

export default function DocumentVersionPage() {
  const params = useParams<{ id: string }>();
  const documentVersionID = Number(params.id);
  const validID = Number.isSafeInteger(documentVersionID) && documentVersionID > 0;

  return (
    <PageShell>
      <PageHeader
        action={
          <Button asChild variant="outline">
            <Link href="/dashboard/contents">
              <ArrowLeft />
              返回采集内容
            </Link>
          </Button>
        }
        description="查看精确正文版本、来源记录和可用性；系统不会把相关性或来源数量解释为事实真实性。"
        eyebrow="Source Evidence"
        title="出处与正文"
      />
      <div className="mt-6">
        {validID ? (
          <DocumentVersionWorkspace documentVersionID={documentVersionID} />
        ) : (
          <Card className="items-center p-10 text-center" role="alert">
            <FileText className="text-muted-foreground" />
            <p className="font-medium">正文版本编号无效</p>
            <p className="text-sm text-muted-foreground">请从引用或预览结果重新进入。</p>
          </Card>
        )}
      </div>
    </PageShell>
  );
}
