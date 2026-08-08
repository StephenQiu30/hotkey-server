"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Check,
  Eye,
  Library,
  Loader2,
  RefreshCw,
  Upload,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  getKnowledgeDocuments,
  getKnowledgeProposals,
  postKnowledgeProposalsIdApply,
  postKnowledgeProposalsIdApprove,
  postKnowledgeProposalsIdReject,
  postKnowledgeReconcile,
} from "@/services/hotkey/hotkey-server/knowledge";

const proposalLabels: Record<string, string> = {
  pending: "待审批",
  approved: "已批准",
  rejected: "已拒绝",
  conflict: "有冲突",
  applied: "已应用",
  failed: "失败",
};

export function KnowledgeArchive() {
  const [documents, setDocuments] = useState<HotKeyAPI.DocumentResponse[]>([]);
  const [proposals, setProposals] = useState<HotKeyAPI.ProposalResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState<number | "reconcile">();
  const [detail, setDetail] = useState<HotKeyAPI.ProposalResponse>();

  const load = useCallback(async () => {
    setLoading(true);
    setError(undefined);
    try {
      const [documentResult, proposalResult] = await Promise.all([
        getKnowledgeDocuments(),
        getKnowledgeProposals({}),
      ]);
      setDocuments(documentResult.data ?? []);
      setProposals(proposalResult.data ?? []);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "知识归档加载失败");
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);

  const change = async (
    proposal: HotKeyAPI.ProposalResponse,
    action: "approve" | "reject" | "apply"
  ) => {
    if (proposal.id == null || proposal.version == null) return;
    setBusy(proposal.id);
    try {
      if (action === "approve")
        await postKnowledgeProposalsIdApprove({
          id: proposal.id,
          version: proposal.version,
        });
      else if (action === "reject")
        await postKnowledgeProposalsIdReject({
          id: proposal.id,
          version: proposal.version,
        });
      else
        await postKnowledgeProposalsIdApply({
          id: proposal.id,
          version: proposal.version,
        });
      toast.success(
        action === "approve"
          ? "提案已批准"
          : action === "reject"
          ? "提案已拒绝"
          : "自动区域已安全写入 Vault"
      );
      await load();
    } catch (reason) {
      toast.error(
        reason instanceof Error ? reason.message : "知识提案操作失败"
      );
    } finally {
      setBusy(undefined);
    }
  };

  const reconcile = async () => {
    setBusy("reconcile");
    try {
      const result = await postKnowledgeReconcile();
      toast.success(
        `对账完成：扫描 ${result.data?.scanned ?? 0}，冲突 ${
          result.data?.conflict ?? 0
        }`
      );
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "Vault 对账失败");
    } finally {
      setBusy(undefined);
    }
  };

  if (error)
    return (
      <Alert variant="destructive">
        <AlertTitle>无法加载知识归档</AlertTitle>
        <AlertDescription className="flex items-center justify-between gap-3">
          <span>{error}</span>
          <Button variant="outline" size="sm" onClick={() => void load()}>
            重试
          </Button>
        </AlertDescription>
      </Alert>
    );
  if (loading)
    return (
      <Card className="space-y-4 p-6">
        {Array.from({ length: 4 }).map((_, index) => (
          <Skeleton key={index} className="h-12 w-full" />
        ))}
      </Card>
    );

  return (
    <section aria-labelledby="knowledge-title" className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2
            id="knowledge-title"
            className="text-2xl font-semibold tracking-[-0.04em]"
          >
            知识归档
          </h2>
          <p className="mt-2 text-sm text-muted-foreground">
            报告发布只生成提案；审批与基线校验通过后才更新 Vault 自动区域。
          </p>
        </div>
        <Button
          variant="outline"
          disabled={busy === "reconcile"}
          onClick={() => void reconcile()}
        >
          {busy === "reconcile" ? (
            <Loader2 className="animate-spin" />
          ) : (
            <RefreshCw />
          )}
          执行对账
        </Button>
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Card className="p-5">
          <p className="text-sm text-muted-foreground">知识文档</p>
          <p className="mt-2 text-2xl font-semibold tabular-nums">
            {documents.length}
          </p>
        </Card>
        <Card className="p-5">
          <p className="text-sm text-muted-foreground">待审批</p>
          <p className="mt-2 text-2xl font-semibold tabular-nums">
            {proposals.filter((item) => item.status === "pending").length}
          </p>
        </Card>
        <Card className="p-5">
          <p className="text-sm text-muted-foreground">冲突</p>
          <p className="mt-2 text-2xl font-semibold tabular-nums">
            {proposals.filter((item) => item.status === "conflict").length}
          </p>
        </Card>
      </div>
      {proposals.length === 0 ? (
        <Card>
          <Empty className="h-64">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Library />
              </EmptyMedia>
              <EmptyTitle>没有知识提案</EmptyTitle>
              <EmptyDescription>
                发布报告后，待审归档提案会出现在这里。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        </Card>
      ) : (
        <Card className="overflow-hidden py-0">
          <Table aria-label="知识提案列表" scrollAreaLabel="知识提案列表">
            <TableHeader>
              <TableRow>
                <TableHead>提案</TableHead>
                <TableHead>状态</TableHead>
                <TableHead className="hidden md:table-cell">文档</TableHead>
                <TableHead className="text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {proposals.map((proposal, index) => (
                <TableRow key={proposal.id ?? `proposal-${index}`}>
                  <TableCell>
                    <p className="font-medium">
                      #{proposal.id} {proposal.diffSummary || "知识变更"}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {proposal.reason}
                    </p>
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        proposal.status === "conflict" ||
                        proposal.status === "failed"
                          ? "destructive"
                          : "secondary"
                      }
                    >
                      {proposalLabels[proposal.status ?? ""] ?? proposal.status}
                    </Badge>
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    #{proposal.documentID} · 基线 v{proposal.baseRevisionNo}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setDetail(proposal)}
                      >
                        <Eye />
                        查看
                      </Button>
                      {proposal.status === "pending" && (
                        <>
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={busy === proposal.id}
                            onClick={() => void change(proposal, "reject")}
                          >
                            <X />
                            拒绝
                          </Button>
                          <Button
                            size="sm"
                            disabled={busy === proposal.id}
                            onClick={() => void change(proposal, "approve")}
                          >
                            <Check />
                            批准
                          </Button>
                        </>
                      )}
                      {proposal.status === "approved" && (
                        <Button
                          size="sm"
                          disabled={busy === proposal.id}
                          onClick={() => void change(proposal, "apply")}
                        >
                          <Upload />
                          应用
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}
      <Dialog
        open={detail != null}
        onOpenChange={(open) => !open && setDetail(undefined)}
      >
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>知识提案 #{detail?.id}</DialogTitle>
            <DialogDescription>
              只会替换 HOTKEY 自动区域，人工笔记保持不变。
            </DialogDescription>
          </DialogHeader>
          <pre className="overflow-x-auto whitespace-pre-wrap rounded-lg border bg-muted/30 p-4 text-xs leading-6">
            {detail?.proposedBody || "无正文"}
          </pre>
        </DialogContent>
      </Dialog>
    </section>
  );
}
