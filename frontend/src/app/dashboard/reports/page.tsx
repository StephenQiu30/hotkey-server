"use client";

import { FileText } from "lucide-react";
import { ReportSubscriptions } from "@/components/notifications/ReportSubscriptions";
import { KnowledgeArchive } from "@/components/reports/KnowledgeArchive";
import { ReportWorkspace } from "@/components/reports/ReportWorkspace";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { UserRole } from "@/lib/domainEnums";
import { useAuthStore } from "@/stores/authStore";

export default function ReportsPage() {
  const role = useAuthStore((state) => state.user?.role);
  const isAdmin = role === UserRole.Admin;
  return <div className="app-page space-y-8">
    <header className="border-b pb-8"><div className="flex items-center gap-2 text-xs font-medium text-primary"><FileText className="h-4 w-4" />可追溯交付</div><h1 className="mt-3 text-3xl font-semibold tracking-[-0.045em]">报告中心</h1><p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">从周期事件快照生成日报与周报，通过邮件或私有 Feed 交付，并以审批提案归档知识。</p></header>
    <Tabs defaultValue="reports" className="space-y-8"><TabsList aria-label="报告中心功能"><TabsTrigger value="reports">报告</TabsTrigger><TabsTrigger value="subscriptions">订阅</TabsTrigger>{isAdmin && <TabsTrigger value="knowledge">知识归档</TabsTrigger>}</TabsList><TabsContent value="reports"><ReportWorkspace /></TabsContent><TabsContent value="subscriptions"><ReportSubscriptions /></TabsContent>{isAdmin && <TabsContent value="knowledge"><KnowledgeArchive /></TabsContent>}</Tabs>
  </div>;
}
