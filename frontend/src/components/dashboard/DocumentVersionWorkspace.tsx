"use client";

import { useCallback, useEffect, useState } from "react";
import { ExternalLink, FileDown, FileText, Loader2, RefreshCw } from "lucide-react";
import { SafeExternalLink } from "@/components/content/SafeExternalLink";
import { SafeMarkdown } from "@/components/content/SafeMarkdown";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  getDocumentVersionsIdCitation,
  getDocumentVersionsIdDocument,
} from "@/services/hotkey/hotkey-server/documentVersions";

const availabilityLabels: Record<string, string> = {
  full_archive: "完整归档",
  partial_archive: "部分归档",
  summary_only: "来源摘要",
  metadata_only: "仅元数据",
  policy_blocked: "权利策略阻止",
  temporarily_unavailable: "暂不可用",
  quarantined: "完整性隔离",
  tombstoned: "已停止保留",
};

function formatDateTime(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return "—";
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function safeBodyFailure(reason: unknown): string {
  if (reason && typeof reason === "object" && "status" in reason) {
    const status = Number((reason as { status?: unknown }).status);
    if (status === 403) return "正文读取被当前权利策略阻止";
    if (status === 404) return "正文投影不存在或已停止保留";
  }
  return "正文投影暂时无法读取";
}

function documentIdentityFailure(
  documentVersionID: number,
  citation: HotKeyAPI.CitationResponseDTO,
  document: HotKeyAPI.VersionedDocumentResponseDTO,
): string | undefined {
  if (
    citation.document_version_id !== documentVersionID ||
    document.citation?.document_version_id !== documentVersionID
  ) {
    return "正文响应与请求版本不一致";
  }
  const citationArtifactSHA = citation.artifact?.sha256;
  const documentArtifactSHA = document.citation?.artifact?.sha256;
  if (
    citationArtifactSHA &&
    documentArtifactSHA &&
    citationArtifactSHA !== documentArtifactSHA
  ) {
    return "正文投影与出处清单不一致";
  }
  return undefined;
}

type DocumentVersionWorkspaceProps = {
  documentVersionID: number;
};

export function DocumentVersionWorkspace({ documentVersionID }: DocumentVersionWorkspaceProps) {
  const [citation, setCitation] = useState<HotKeyAPI.CitationResponseDTO>();
  const [document, setDocument] = useState<HotKeyAPI.VersionedDocumentResponseDTO>();
  const [bodyFailure, setBodyFailure] = useState<string>();
  const [loadFailure, setLoadFailure] = useState<string>();
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    setLoadFailure(undefined);
    setBodyFailure(undefined);
    setCitation(undefined);
    setDocument(undefined);

    const [citationResult, documentResult] = await Promise.allSettled([
      getDocumentVersionsIdCitation({ id: documentVersionID }),
      getDocumentVersionsIdDocument({ id: documentVersionID }),
    ]);
    if (citationResult.status === "rejected" || !citationResult.value.data) {
      const reason = citationResult.status === "rejected" ? citationResult.reason : undefined;
      setLoadFailure(reason instanceof Error ? reason.message : "出处信息暂时无法读取");
      setLoading(false);
      return;
    }

    const nextCitation = citationResult.value.data;
    if (nextCitation.document_version_id !== documentVersionID) {
      setLoadFailure("出处响应与请求版本不一致");
      setLoading(false);
      return;
    }
    setCitation(nextCitation);

    if (documentResult.status === "rejected" || !documentResult.value.data) {
      const reason = documentResult.status === "rejected" ? documentResult.reason : undefined;
      setBodyFailure(safeBodyFailure(reason));
      setLoading(false);
      return;
    }
    const nextDocument = documentResult.value.data;
    const identityFailure = documentIdentityFailure(documentVersionID, nextCitation, nextDocument);
    if (identityFailure) {
      setBodyFailure(identityFailure);
    } else if (!nextDocument.markdown) {
      setBodyFailure("正文投影为空，系统不会用标题或摘要替代");
    } else {
      setDocument(nextDocument);
    }
    setLoading(false);
  }, [documentVersionID]);

  useEffect(() => {
    void load();
  }, [load]);

  if (loading) {
    return (
      <div className="flex min-h-72 items-center justify-center" aria-label="加载正文版本">
        <Loader2 className="animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (loadFailure || !citation) {
    return (
      <Card className="items-center p-10 text-center" role="alert">
        <p className="font-medium">出处信息加载失败</p>
        <p className="text-sm text-muted-foreground">{loadFailure || "出处响应为空"}</p>
        <Button onClick={() => void load()} type="button" variant="outline">
          <RefreshCw />
          重试
        </Button>
      </Card>
    );
  }

  return (
    <article className="document-print-root mx-auto w-full max-w-[960px]">
      <header className="document-header border-b border-border pb-7">
        <div className="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <p className="eyebrow">Immutable document version</p>
              <Badge variant="outline">
                {availabilityLabels[citation.availability ?? ""] ?? citation.availability ?? "未知状态"}
              </Badge>
            </div>
            <h1 className="mt-3 text-2xl font-semibold sm:text-3xl">
              {citation.title || `正文版本 #${documentVersionID}`}
            </h1>
            <p className="mt-3 text-sm text-muted-foreground">
              <span>{citation.source_name || "未知来源"}</span>
              {citation.source_type ? <span> · {citation.source_type}</span> : null}
              {citation.author ? <span> · {citation.author}</span> : null}
            </p>
            <p className="mt-2 text-xs text-muted-foreground">
              发布于 {formatDateTime(citation.published_at)} · 采集于 {formatDateTime(citation.captured_at)}
            </p>
          </div>
          <div className="document-actions flex shrink-0 flex-wrap gap-2">
            <Button disabled={!document?.markdown} onClick={() => window.print()} type="button">
              <FileDown />
              打印 / 保存 PDF
            </Button>
          </div>
        </div>

        <div className="document-scope mt-6 space-y-2 rounded-md border border-border bg-muted/30 px-4 py-3 text-xs leading-5 text-muted-foreground">
          <p>
            正文来自已归档的精确来源观察，并绑定不可变 DocumentVersion。出处和完整性记录不等于对报道真实性作出判断。
          </p>
          <p>
            正文类型：<span className="font-medium text-foreground">{citation.body_origin || "未知"} · {citation.completeness || "未知"}</span>
          </p>
          <p className="mono">Document #{citation.document_id ?? "—"} · Version #{citation.document_version_id ?? "—"}</p>
        </div>

        <nav aria-label="正文出处链接" className="mt-4 flex flex-wrap gap-2">
          {citation.canonical_url ? (
            <Button asChild size="sm" variant="outline">
              <SafeExternalLink href={citation.canonical_url}>
                发布页 <ExternalLink />
              </SafeExternalLink>
            </Button>
          ) : null}
          {citation.source_record_url ? (
            <Button asChild size="sm" variant="outline">
              <SafeExternalLink href={citation.source_record_url}>
                来源记录 <ExternalLink />
              </SafeExternalLink>
            </Button>
          ) : null}
          {citation.discussion_url ? (
            <Button asChild size="sm" variant="outline">
              <SafeExternalLink href={citation.discussion_url}>
                讨论页 <ExternalLink />
              </SafeExternalLink>
            </Button>
          ) : null}
        </nav>
      </header>

      {document?.markdown ? (
        <SafeMarkdown className="py-8" markdown={document.markdown} />
      ) : (
        <Card className="my-8">
          <Empty className="min-h-64 border-0">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <FileText />
              </EmptyMedia>
              <EmptyTitle className="text-base">正文当前不可读取</EmptyTitle>
              <EmptyDescription>
                {bodyFailure || "当前版本没有可安全展示的 Markdown 投影。"}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        </Card>
      )}

      <footer className="document-footer space-y-2 border-t border-border py-5 text-xs text-muted-foreground">
        {citation.unavailable_reason ? <p>{citation.unavailable_reason}</p> : null}
        {citation.locator_availability === "unavailable" ? (
          <p>
            引用定位暂不可用
            {citation.locator_unavailable_reason ? (
              <>
                ：<span>{citation.locator_unavailable_reason}</span>
              </>
            ) : null}
          </p>
        ) : null}
        {citation.artifact?.sha256 ? (
          <p className="mono">Markdown SHA-256 {citation.artifact.sha256.slice(0, 16)}…</p>
        ) : null}
      </footer>
    </article>
  );
}
