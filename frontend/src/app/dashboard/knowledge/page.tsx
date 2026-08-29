"use client";

import { useCallback, useEffect, useState } from "react";
import { FileText, FolderSync, LibraryBig, Loader2, RefreshCw, ShieldAlert } from "lucide-react";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { CursorPagination } from "@/components/dashboard/CursorPagination";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Surface } from "@/components/ui/surface";
import { HotKeyAPIError } from "@/lib/request";
import { UserRole } from "@/lib/domainEnums";
import {
  getKnowledgeDocuments,
  getKnowledgeProposals,
  postKnowledgeProposalsIdApply,
  postKnowledgeProposalsIdApprove,
  postKnowledgeProposalsIdReject,
  postKnowledgeReconcile,
} from "@/services/hotkey/hotkey-server/knowledge";
import { useAuthStore } from "@/stores/authStore";
import { PageShell } from "@/layouts/PageShell";

const documentStatus: Readonly<Record<string, string>> = {
  planned: "待投影",
  active: "已同步",
  conflict: "冲突",
  archived: "已归档",
};

const proposalStatus: Readonly<Record<string, string>> = {
  pending: "待审批",
  approved: "已批准",
  rejected: "已驳回",
  applied: "已应用",
  conflict: "冲突",
};

const PAGE_SIZE = 10;

export default function KnowledgePage() {
  const role = useAuthStore((state) => state.user?.role);
  const publisher = role === UserRole.Editor || role === UserRole.Admin;
  const admin = role === UserRole.Admin;
  const [documents, setDocuments] = useState<HotKeyAPI.DocumentResponse[]>([]);
  const [proposals, setProposals] = useState<HotKeyAPI.ProposalResponse[]>([]);
  const [documentPage, setDocumentPage] = useState(1);
  const [documentCursors, setDocumentCursors] = useState<(string | undefined)[]>([undefined]);
  const [documentNextCursor, setDocumentNextCursor] = useState<string>();
  const [proposalPage, setProposalPage] = useState(1);
  const [proposalCursors, setProposalCursors] = useState<(string | undefined)[]>([undefined]);
  const [proposalNextCursor, setProposalNextCursor] = useState<string>();
  const [reconciliation, setReconciliation] = useState<HotKeyAPI.ReconciliationResponse>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string>();
  const [error, setError] = useState<string>();
  const [permissionDenied, setPermissionDenied] = useState(false);

  const loadDocuments = useCallback(async (cursor: string | undefined, page: number) => {
    const params: HotKeyAPI.getKnowledgeDocumentsParams = { limit: PAGE_SIZE };
    if (cursor) params.cursor = cursor;
    const result = await getKnowledgeDocuments(params);
    setDocuments(result.data?.items ?? []);
    setDocumentNextCursor(result.data?.next_cursor);
    setDocumentPage(page);
  }, []);

  const loadProposals = useCallback(async (cursor: string | undefined, page: number) => {
    const params: HotKeyAPI.getKnowledgeProposalsParams = { limit: PAGE_SIZE };
    if (cursor) params.cursor = cursor;
    const result = await getKnowledgeProposals(params);
    setProposals(result.data?.items ?? []);
    setProposalNextCursor(result.data?.next_cursor);
    setProposalPage(page);
  }, []);

  const loadInitial = useCallback(async () => {
    if (!publisher) {
      setLoading(false);
      setPermissionDenied(true);
      return;
    }
    setLoading(true);
    setError(undefined);
    setPermissionDenied(false);
    try {
      setDocumentCursors([undefined]);
      setProposalCursors([undefined]);
      await Promise.all([loadDocuments(undefined, 1), loadProposals(undefined, 1)]);
    } catch (reason) {
      setPermissionDenied(reason instanceof HotKeyAPIError && reason.status === 403);
      setError(reason instanceof Error ? reason.message : "知识投影加载失败");
    } finally {
      setLoading(false);
    }
  }, [loadDocuments, loadProposals, publisher]);

  useEffect(() => {
    void loadInitial();
  }, [loadInitial]);

  async function refreshCurrent() {
    setLoading(true);
    setError(undefined);
    try {
      await Promise.all([
        loadDocuments(documentCursors[documentPage - 1], documentPage),
        loadProposals(proposalCursors[proposalPage - 1], proposalPage),
      ]);
    } catch (reason) {
      setPermissionDenied(reason instanceof HotKeyAPIError && reason.status === 403);
      setError(reason instanceof Error ? reason.message : "知识投影加载失败");
    } finally {
      setLoading(false);
    }
  }

  async function mutateProposal(
    label: string,
    proposal: HotKeyAPI.ProposalResponse,
    operation: () => Promise<{ data?: HotKeyAPI.ProposalResponse | HotKeyAPI.DocumentResponse }>,
  ) {
    setBusy(label);
    setError(undefined);
    try {
      const result = await operation();
      if ("documentID" in (result.data ?? {})) {
        const next = result.data as HotKeyAPI.ProposalResponse;
        setProposals((current) => current.map((item) => item.id === proposal.id ? next : item));
      } else {
        const next = result.data as HotKeyAPI.DocumentResponse | undefined;
        if (next?.id) setDocuments((current) => current.map((item) => item.id === next.id ? next : item));
        await refreshCurrent();
      }
    } catch (reason) {
      setPermissionDenied(reason instanceof HotKeyAPIError && reason.status === 403);
      setError(reason instanceof Error ? reason.message : "知识投影操作失败");
    } finally {
      setBusy(undefined);
    }
  }

  async function reconcile() {
    setBusy("reconcile");
    setError(undefined);
    try {
      const result = await postKnowledgeReconcile();
      setReconciliation(result.data);
    } catch (reason) {
      setPermissionDenied(reason instanceof HotKeyAPIError && reason.status === 403);
      setError(reason instanceof Error ? reason.message : "Vault 对账失败");
    } finally {
      setBusy(undefined);
    }
  }

  async function changeDocumentPage(cursor: string | undefined, page: number) {
    setBusy("documents-page");
    setError(undefined);
    try {
      await loadDocuments(cursor, page);
    } catch (reason) {
      setPermissionDenied(reason instanceof HotKeyAPIError && reason.status === 403);
      setError(reason instanceof Error ? reason.message : "知识文档加载失败");
    } finally {
      setBusy(undefined);
    }
  }

  async function changeProposalPage(cursor: string | undefined, page: number) {
    setBusy("proposals-page");
    setError(undefined);
    try {
      await loadProposals(cursor, page);
    } catch (reason) {
      setPermissionDenied(reason instanceof HotKeyAPIError && reason.status === 403);
      setError(reason instanceof Error ? reason.message : "知识提案加载失败");
    } finally {
      setBusy(undefined);
    }
  }

  function nextDocumentPage() {
    if (!documentNextCursor) return;
    const targetPage = documentPage + 1;
    setDocumentCursors((current) => {
      const updated = current.slice(0, targetPage - 1);
      updated[targetPage - 1] = documentNextCursor;
      return updated;
    });
    void changeDocumentPage(documentNextCursor, targetPage);
  }

  function nextProposalPage() {
    if (!proposalNextCursor) return;
    const targetPage = proposalPage + 1;
    setProposalCursors((current) => {
      const updated = current.slice(0, targetPage - 1);
      updated[targetPage - 1] = proposalNextCursor;
      return updated;
    });
    void changeProposalPage(proposalNextCursor, targetPage);
  }

  if (!publisher && permissionDenied) {
    return (
      <PageShell>
        <PageHeader eyebrow="KNOWLEDGE VAULT" title="知识投影" description="受控发布界面仅对 Editor 与 Admin 开放。" />
        <Alert aria-label="知识投影权限不足" className="mt-6">
          <ShieldAlert />
          <AlertTitle>权限不足</AlertTitle>
          <AlertDescription>当前角色可以通过全文检索读取已发布知识，但不能查看或修改 Vault 投影治理状态。</AlertDescription>
        </Alert>
      </PageShell>
    );
  }

  return (
    <PageShell>
      <PageHeader
        eyebrow="KNOWLEDGE VAULT"
        title="知识投影"
        description="只管理稳定业务 ID、受控相对路径和自动区域 Revision；任何操作都不能提交宿主机绝对路径。"
        action={
          <div className="flex flex-wrap gap-2">
            {admin ? (
              <Button variant="outline" onClick={() => void reconcile()} disabled={Boolean(busy)}>
                {busy === "reconcile" ? <Loader2 className="animate-spin" /> : <FolderSync />}执行 Vault 对账
              </Button>
            ) : null}
            <Button variant="outline" onClick={() => void refreshCurrent()} disabled={loading || Boolean(busy)}><RefreshCw />刷新</Button>
          </div>
        }
      />

      {permissionDenied ? (
        <Alert variant="destructive" className="mt-6" aria-label="知识操作权限不足">
          <AlertTitle>没有知识投影操作权限</AlertTitle>
          <AlertDescription>服务端拒绝了本次操作，现有投影事实保持不变。</AlertDescription>
        </Alert>
      ) : error ? (
        <Alert variant="destructive" className="mt-6">
          <AlertTitle>知识投影请求失败</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      {reconciliation ? (
        <Alert className="mt-6" aria-label="Vault 对账结果">
          <AlertTitle>对账完成</AlertTitle>
          <AlertDescription>扫描 {reconciliation.scanned ?? 0} 个投影，发现 {reconciliation.conflict ?? 0} 个冲突和 {reconciliation.changed ?? 0} 个额外文件。</AlertDescription>
        </Alert>
      ) : null}

      {loading ? (
        <section aria-label="正在加载知识投影" className="mt-6 grid gap-4 md:grid-cols-2">
          <Skeleton className="h-72" />
          <Skeleton className="h-72" />
        </section>
      ) : (
        <div className="mt-6 grid min-w-0 gap-5 xl:grid-cols-2">
          <Card className="min-w-0">
            <CardHeader><CardTitle role="heading" aria-level={2}>当前知识文档</CardTitle></CardHeader>
            <CardContent className="space-y-3">
              {documents.length === 0 ? (
                <Empty className="h-56">
                  <EmptyHeader>
                    <EmptyMedia variant="icon"><LibraryBig /></EmptyMedia>
                    <EmptyTitle role="heading" aria-level={3}>尚未发布知识</EmptyTitle>
                    <EmptyDescription>批准的报告或事件投影进入 Vault 后会显示在这里。</EmptyDescription>
                  </EmptyHeader>
                </Empty>
              ) : documents.map((document) => (
                <Surface key={document.id} asChild variant="ring">
                  <article className="p-4">
                    <div className="flex flex-wrap items-center gap-2">
                      <strong>知识文档 #{document.id}</strong>
                      <Badge variant={document.status === "conflict" ? "destructive" : "outline"}>{documentStatus[document.status ?? ""] ?? document.status}</Badge>
                      <span className="ml-auto text-xs text-muted-foreground">Revision {document.revisionNo ?? 0}</span>
                    </div>
                    <p className="mt-2 break-all font-mono text-xs text-muted-foreground">{document.vaultPath}</p>
                    <p className="mt-2 text-xs text-muted-foreground">稳定来源：{document.type} {document.reportID ?? document.eventID ?? document.topicID}</p>
                  </article>
                </Surface>
              ))}
            </CardContent>
            <CursorPagination
              page={documentPage}
              pageSize={PAGE_SIZE}
              hasNext={Boolean(documentNextCursor)}
              loading={busy === "documents-page"}
              onPrevious={() => void changeDocumentPage(documentCursors[documentPage - 2], documentPage - 1)}
              onNext={nextDocumentPage}
            />
          </Card>

          <Card className="min-w-0">
            <CardHeader><CardTitle role="heading" aria-level={2}>发布提案与冲突</CardTitle></CardHeader>
            <CardContent className="space-y-3">
              {proposals.length === 0 ? (
                <Empty className="h-56">
                  <EmptyHeader>
                    <EmptyMedia variant="icon"><FileText /></EmptyMedia>
                    <EmptyTitle role="heading" aria-level={3}>没有待处理提案</EmptyTitle>
                    <EmptyDescription>自动区域更新或人工冲突会生成受版本围栏保护的提案。</EmptyDescription>
                  </EmptyHeader>
                </Empty>
              ) : proposals.map((proposal) => (
                <Surface key={proposal.id} asChild variant="ring">
                  <article className="p-4">
                    <div className="flex flex-wrap items-center gap-2">
                      <strong>提案 #{proposal.id}</strong>
                      <Badge variant={proposal.status === "conflict" ? "destructive" : "outline"}>{proposalStatus[proposal.status ?? ""] ?? proposal.status}</Badge>
                      <span className="ml-auto text-xs text-muted-foreground">文档 #{proposal.documentID}</span>
                    </div>
                    <p className="mt-2 text-sm text-muted-foreground">{proposal.reason || proposal.diffSummary || "自动区域更新"}</p>
                    <div className="mt-3 flex flex-wrap gap-2">
                      {proposal.status === "pending" ? (
                        <>
                          <Button size="sm" disabled={Boolean(busy)} onClick={() => void mutateProposal(`approve-${proposal.id}`, proposal, () => postKnowledgeProposalsIdApprove({ id: proposal.id!, version: proposal.version! }))}>
                            {busy === `approve-${proposal.id}` ? <Loader2 className="animate-spin" /> : null}批准提案
                          </Button>
                          <Button size="sm" variant="outline" disabled={Boolean(busy)} onClick={() => void mutateProposal(`reject-${proposal.id}`, proposal, () => postKnowledgeProposalsIdReject({ id: proposal.id!, version: proposal.version! }))}>驳回提案</Button>
                        </>
                      ) : null}
                      {proposal.status === "approved" ? (
                        <Button size="sm" disabled={Boolean(busy)} onClick={() => void mutateProposal(`apply-${proposal.id}`, proposal, () => postKnowledgeProposalsIdApply({ id: proposal.id!, version: proposal.version! }))}>
                          {busy === `apply-${proposal.id}` ? <Loader2 className="animate-spin" /> : null}原子发布到 Vault
                        </Button>
                      ) : null}
                    </div>
                  </article>
                </Surface>
              ))}
            </CardContent>
            <CursorPagination
              page={proposalPage}
              pageSize={PAGE_SIZE}
              hasNext={Boolean(proposalNextCursor)}
              loading={busy === "proposals-page"}
              onPrevious={() => void changeProposalPage(proposalCursors[proposalPage - 2], proposalPage - 1)}
              onNext={nextProposalPage}
            />
          </Card>
        </div>
      )}
    </PageShell>
  );
}
