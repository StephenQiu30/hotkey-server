"use client";

import { useCallback, useEffect, useState } from "react";
import { ArrowLeft, Loader2, RefreshCw, ShieldAlert } from "lucide-react";
import { useParams, useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ContentDocumentViewer } from "@/components/dashboard/ContentDocumentViewer";
import { ConfirmDeleteDialog } from "@/components/dashboard/ConfirmDeleteDialog";
import { deleteContentsId, getContentsIdDocument } from "@/services/hotkey/hotkey-server/contents";
import { useAuthStore } from "@/stores/authStore";
import { UserRole } from "@/lib/domainEnums";

type DetailError = 403 | 404 | 503;

const errorCode = (reason: unknown): DetailError => {
  if (reason && typeof reason === "object" && "code" in reason) {
    const code = Number((reason as { code?: unknown }).code);
    if (code === 403 || code === 404 || code === 503) return code;
  }
  return 503;
};

const errorCopy: Record<DetailError, { title: string; description: string }> = {
  403: {
    title: "无权查看该归档内容",
    description: "当前账号没有读取此内容的权限，请返回采集内容或联系管理员。",
  },
  404: {
    title: "内容不存在",
    description: "内容可能已被删除，或者地址中的内容编号无效。",
  },
  503: {
    title: "归档暂不可用",
    description: "归档存储暂时无法读取，请稍后重试。页面不会展示缓存中的旧正文。",
  },
};

export default function ContentDetailPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const user = useAuthStore((state) => state.user);
  const canManage = user?.role === UserRole.Editor || user?.role === UserRole.Admin;
  const id = Number(params.id);
  const validId = Number.isSafeInteger(id) && id > 0;
  const [document, setDocument] = useState<HotKeyAPI.ContentDocumentResponse>();
  const [error, setError] = useState<DetailError>();
  const [loading, setLoading] = useState(validId);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const load = useCallback(async () => {
    if (!validId) {
      setDocument(undefined);
      setError(404);
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(undefined);
    setDocument(undefined);
    try {
      const result = await getContentsIdDocument({ id });
      if (!result.data) throw Object.assign(new Error("empty response"), { code: 503 });
      setDocument(result.data);
    } catch (reason) {
      setDocument(undefined);
      setError(errorCode(reason));
    } finally {
      setLoading(false);
    }
  }, [id, validId]);

  useEffect(() => {
    void load();
  }, [load]);

  const deleteContent = async () => {
    if (!canManage || document?.content_id == null) return;
    setDeleting(true);
    try {
      await deleteContentsId({ id: document.content_id });
      setDeleteOpen(false);
      toast.success("内容已删除，归档证据将按生命周期清理");
      router.push("/dashboard/contents");
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "内容删除失败");
    } finally {
      setDeleting(false);
    }
  };

  if (loading) {
    return (
      <div className="flex min-h-[calc(100vh-60px)] items-center justify-center">
        <Loader2 aria-label="加载归档内容" className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error || !document) {
    const copy = errorCopy[error ?? 503];
    return (
      <div className="app-page">
        <div className="panel mx-auto flex min-h-80 max-w-2xl flex-col items-center justify-center px-6 text-center">
          <ShieldAlert className="mb-4 h-6 w-6 text-muted-foreground" />
          <h1 className="text-xl font-semibold">{copy.title}</h1>
          <p className="mt-2 max-w-md text-sm leading-6 text-muted-foreground">{copy.description}</p>
          <div className="mt-6 flex flex-wrap justify-center gap-2">
            <Button asChild variant="outline" className="gap-2">
              <a href="/dashboard/contents">
                <ArrowLeft className="h-4 w-4" /> 返回采集内容
              </a>
            </Button>
            {error === 503 ? (
              <Button onClick={load} className="gap-2">
                <RefreshCw className="h-4 w-4" /> 重试
              </Button>
            ) : null}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="app-page document-page">
      <ContentDocumentViewer
        canManage={canManage}
        deleting={deleting}
        document={document}
        onDelete={() => setDeleteOpen(true)}
      />
      <ConfirmDeleteDialog
        description="内容会从采集列表和后续候选中移除；系统保留生命周期墓碑，并清理已归档的 Markdown 证据。"
        loading={deleting}
        onConfirm={deleteContent}
        onOpenChange={setDeleteOpen}
        open={deleteOpen}
        resourceName={document.title || `内容 #${document.content_id ?? ""}`}
        title="删除采集内容"
      />
    </div>
  );
}
