"use client";

import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ExternalLink, FileDown, FileText, Highlighter, Loader2, RefreshCw } from "lucide-react";
import { SafeExternalLink } from "@/components/content/SafeExternalLink";
import { SafeMarkdown } from "@/components/content/SafeMarkdown";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
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
  postDocumentVersionsIdTextQuoteSelectors,
} from "@/services/hotkey/hotkey-server/documentVersions";
import { useAuthStore } from "@/stores/authStore";

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
  if (!citationArtifactSHA || citationArtifactSHA !== documentArtifactSHA) {
    return "正文投影与出处清单不一致";
  }
  const citationAnchorMapSHA = citation.artifact?.anchor_map?.anchor_map_sha256;
  const documentAnchorMapSHA = document.citation?.artifact?.anchor_map?.anchor_map_sha256;
  if (!citationAnchorMapSHA || citationAnchorMapSHA !== documentAnchorMapSHA) {
    return "正文锚点映射与出处清单不一致";
  }
  return undefined;
}

function PartyIdentity({ party }: { party: HotKeyAPI.CitationPartyResponseDTO }) {
  const label = party.display_name || party.external_id || "未命名主体";
  const identity = [party.identity_namespace, party.external_id].filter(Boolean).join(":");

  return (
    <div className="min-w-0">
      {party.homepage_url ? (
        <SafeExternalLink
          className="inline-flex max-w-full items-center gap-1 font-medium text-foreground underline-offset-4 hover:underline"
          href={party.homepage_url}
        >
          <span className="truncate">{label}</span>
          <ExternalLink className="size-3 shrink-0" />
        </SafeExternalLink>
      ) : (
        <p className="font-medium text-foreground">{label}</p>
      )}
      {identity ? <p className="mono mt-1 break-all text-[11px]">{identity}</p> : null}
    </div>
  );
}

type DocumentVersionWorkspaceProps = {
  documentVersionID: number;
};

export function DocumentVersionWorkspace({ documentVersionID }: DocumentVersionWorkspaceProps) {
  const role = useAuthStore((state) => state.user?.role);
  const canQuote = role === "editor" || role === "admin";
  const [citation, setCitation] = useState<HotKeyAPI.CitationResponseDTO>();
  const [document, setDocument] = useState<HotKeyAPI.VersionedDocumentResponseDTO>();
  const [bodyFailure, setBodyFailure] = useState<string>();
  const [loadFailure, setLoadFailure] = useState<string>();
  const [loading, setLoading] = useState(true);
  const [exactQuote, setExactQuote] = useState("");
  const [quoteSelector, setQuoteSelector] = useState<HotKeyAPI.TextQuoteSelectorResponseDTO>();
  const [quoteFailure, setQuoteFailure] = useState<string>();
  const [quoteBusy, setQuoteBusy] = useState(false);
  const documentBody = useRef<HTMLDivElement>(null);
  const markdownAnchors = useMemo(
    () =>
      document?.citation?.artifact?.anchor_map?.blocks?.map((block) => ({
        ordinal: block.ordinal ?? -1,
        markdownAnchor: block.markdown_anchor ?? "",
      })),
    [document?.citation?.artifact?.anchor_map?.blocks],
  );

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

  const captureSelection = () => {
    const selection = window.getSelection();
    const anchorNode = selection?.anchorNode;
    const selectedText = selection?.toString().trim() ?? "";
    if (!anchorNode || !documentBody.current?.contains(anchorNode) || !selectedText) {
      setQuoteFailure("请先在归档正文中选择一段连续文字");
      return;
    }
    setExactQuote(selectedText);
    setQuoteFailure(undefined);
    setQuoteSelector(undefined);
  };

  const createQuoteSelector = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const plaintextSHA256 = citation?.content_sha256;
    if (!plaintextSHA256) {
      setQuoteFailure("当前正文没有可安全引用的 plaintext 身份");
      return;
    }
    setQuoteBusy(true);
    setQuoteFailure(undefined);
    try {
      const result = await postDocumentVersionsIdTextQuoteSelectors(
        { id: documentVersionID },
        {
          exact_quote: exactQuote.trim(),
          plaintext_sha256: plaintextSHA256,
          normalization_version: citation.artifact?.anchor_map?.normalization_version ?? "nfc-lf-collapse-space-v1",
        },
        {
          headers: {
            "Content-Type": "application/json",
            "If-Match": `"${plaintextSHA256}"`,
            "Idempotency-Key": crypto.randomUUID(),
          },
        },
      );
      if (!result.data?.id) throw new Error("引用选择器响应为空");
      setQuoteSelector(result.data);
    } catch (reason) {
      setQuoteFailure(reason instanceof Error ? reason.message : "引用选择器创建失败");
    } finally {
      setQuoteBusy(false);
    }
  };

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

        <section aria-labelledby="document-parties-title" className="mt-4 rounded-md border border-border p-4">
          <h2 className="text-sm font-semibold" id="document-parties-title">出处主体</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            仅展示来源显式提供并绑定到本次归档证据的主体；缺失信息不会通过域名、作者或模型推断。
          </p>
          <dl className="mt-4 grid gap-4 text-sm sm:grid-cols-3">
            <div>
              <dt className="mb-1 text-xs text-muted-foreground">发布者</dt>
              <dd>
                {citation.publisher_party ? (
                  <PartyIdentity party={citation.publisher_party} />
                ) : (
                  <span className="text-muted-foreground">发布者信息未提供</span>
                )}
              </dd>
            </div>
            <div>
              <dt className="mb-1 text-xs text-muted-foreground">内容源主体</dt>
              <dd>
                {citation.content_origin ? (
                  <PartyIdentity party={citation.content_origin} />
                ) : (
                  <span className="text-muted-foreground">内容源主体未提供</span>
                )}
              </dd>
            </div>
            <div>
              <dt className="mb-1 text-xs text-muted-foreground">分发方</dt>
              <dd className="space-y-2">
                {citation.distributors?.length ? (
                  citation.distributors.map((party, index) => (
                    <PartyIdentity
                      key={`${party.identity_namespace ?? "unknown"}:${party.external_id ?? index}`}
                      party={party}
                    />
                  ))
                ) : (
                  <span className="text-muted-foreground">分发方信息未提供</span>
                )}
              </dd>
            </div>
          </dl>
        </section>

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
        <div ref={documentBody}>
          <SafeMarkdown anchors={markdownAnchors} className="py-8" markdown={document.markdown} />
        </div>
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

      {canQuote && document?.markdown && citation.content_sha256 ? (
        <section aria-labelledby="quote-selector-title" className="document-actions mb-8 rounded-lg border border-border bg-muted/20 p-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <h2 className="font-semibold" id="quote-selector-title">创建精确正文引用</h2>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">先在正文中选择文字，再由服务端在 immutable NFC plaintext 中唯一定位并计算 UTF-8 字节范围。</p>
            </div>
            <Button onClick={captureSelection} type="button" variant="outline"><Highlighter />使用选中文字</Button>
          </div>
          <form className="mt-4 space-y-3" onSubmit={createQuoteSelector}>
            <Label htmlFor="document-exact-quote">精确摘录</Label>
            <Textarea className="min-h-28 leading-6" id="document-exact-quote" maxLength={4096} onChange={(event) => { setExactQuote(event.target.value); setQuoteSelector(undefined); }} required value={exactQuote} />
            {quoteFailure ? <p className="text-sm text-destructive" role="alert">{quoteFailure}</p> : null}
            {quoteSelector ? <p className="rounded-md border border-border bg-background px-3 py-2 text-sm">引用选择器 #{quoteSelector.id} 已创建 · UTF-8 {quoteSelector.utf8_byte_start}–{quoteSelector.utf8_byte_end}{quoteSelector.markdown_anchor ? ` · #${quoteSelector.markdown_anchor}` : ""}</p> : null}
            <Button disabled={quoteBusy || !exactQuote.trim()} type="submit">{quoteBusy ? <Loader2 className="animate-spin" /> : null}生成引用选择器</Button>
          </form>
        </section>
      ) : null}

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
        {citation.artifact?.anchor_map ? (
          <p className="mono">
            锚点映射 {citation.artifact.anchor_map.anchor_map_profile_version} ·{" "}
            {citation.artifact.anchor_map.blocks?.length ?? 0} blocks
          </p>
        ) : null}
      </footer>
    </article>
  );
}
