"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { ExternalLink, LockKeyhole, ShieldCheck } from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@/components/ui/tabs";
import { UserRole } from "@/lib/domainEnums";
import { getRadarEventTitle } from "@/lib/radarPresentation";

type GovernanceEvent = HotKeyAPI.RadarEventResponse;

type EventGovernancePanelProps = {
  event: GovernanceEvent;
  events: GovernanceEvent[];
  members: HotKeyAPI.EventMemberResponse[];
  role?: string;
  loading: boolean;
  error: boolean;
  busy: boolean;
  operationError?: string;
  onToggleLock(member: HotKeyAPI.EventMemberResponse): Promise<void>;
  onLifecycle(status: string): Promise<void>;
  onMerge(target: GovernanceEvent): Promise<void>;
  onSplit(members: HotKeyAPI.EventMemberResponse[]): Promise<void>;
};

const lifecycleLabels: Record<string, string> = {
  detected: "待确认",
  active: "活跃",
  cooling: "降温",
  closed: "已关闭",
  archived: "已归档",
  rejected: "已排除",
  merged: "已合并",
};

const transitions: Record<string, string[]> = {
  detected: ["active", "rejected"],
  active: ["cooling", "closed", "rejected"],
  cooling: ["active", "closed", "rejected"],
  closed: ["active", "archived", "rejected"],
};

export function EventGovernancePanel({
  event,
  events,
  members,
  role,
  loading,
  error,
  busy,
  operationError,
  onToggleLock,
  onLifecycle,
  onMerge,
  onSplit,
}: EventGovernancePanelProps) {
  const canLock = role === UserRole.Editor || role === UserRole.Admin;
  const isAdmin = role === UserRole.Admin;
  const isTerminal = ["archived", "rejected", "merged"].includes(
    event.lifecycle_status ?? ""
  );
  const [lifecycle, setLifecycle] = useState("");
  const [targetId, setTargetId] = useState("");
  const [splitIds, setSplitIds] = useState<number[]>([]);

  useEffect(() => {
    setLifecycle("");
    setTargetId("");
    setSplitIds([]);
  }, [event.event_id, event.lifecycle_status]);

  const mergeTargets = useMemo(
    () =>
      events.filter(
        (item) =>
          item.event_id !== event.event_id &&
          item.version &&
          !["archived", "rejected", "merged"].includes(
            item.lifecycle_status ?? ""
          )
      ),
    [event.event_id, events]
  );
  const selectedTarget = mergeTargets.find(
    (item) => String(item.event_id) === targetId
  );
  const selectedMembers = members.filter((member) =>
    splitIds.includes(member.content_id ?? -1)
  );
  const splitAllowed =
    selectedMembers.length > 0 && selectedMembers.length < members.length;

  return (
    <section
      className="border-t pt-5"
      aria-labelledby="event-governance-heading"
    >
      <div className="flex items-center justify-between gap-3">
        <h3 id="event-governance-heading" className="text-sm font-semibold">
          聚类与治理
        </h3>
        <Badge variant="secondary" className="font-normal">
          {lifecycleLabels[event.lifecycle_status ?? ""] ?? "未知状态"}
        </Badge>
      </div>

      {loading ? (
        <div className="mt-4 space-y-2" aria-label="事件成员加载中">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      ) : error ? (
        <Alert variant="destructive" className="mt-4">
          <LockKeyhole className="h-4 w-4" />
          <AlertTitle>聚类成员暂时不可用</AlertTitle>
          <AlertDescription>其他事件信息不受影响。</AlertDescription>
        </Alert>
      ) : (
        <div className="mt-4">
          {members.length ? (
            <ScrollArea
              aria-label="聚类成员列表"
              className="h-96 pr-3"
              role="region"
            >
              <div className="space-y-2">
                {members.map((member) => (
                  <div key={member.id} className="rounded-lg bg-muted/45 p-3">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <Link
                          href={`/dashboard/contents/${member.content_id}`}
                          className="inline-flex items-center gap-1 text-sm font-medium no-underline hover:underline"
                        >
                          内容 #{member.content_id}
                          <ExternalLink className="h-3.5 w-3.5" />
                        </Link>
                        <p className="mt-1 text-xs text-muted-foreground">
                          匹配 {Math.round(member.membership_score ?? 0)} ·{" "}
                          {member.evidence_role || "supporting"} ·{" "}
                          {member.origin || "rule"}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        {member.manual_locked ? (
                          <Badge variant="outline">已锁定</Badge>
                        ) : null}
                        {canLock ? (
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={
                              busy ||
                              isTerminal ||
                              !member.version ||
                              !member.content_id
                            }
                            aria-label={`${
                              member.manual_locked ? "解锁" : "锁定"
                            }内容 ${member.content_id}`}
                            onClick={() => void onToggleLock(member)}
                          >
                            {member.manual_locked ? "解锁" : "锁定"}
                          </Button>
                        ) : null}
                      </div>
                    </div>
                    {isAdmin && members.length > 1 ? (
                      <label className="mt-3 flex cursor-pointer items-center gap-2 text-xs text-muted-foreground">
                        <Checkbox
                          checked={splitIds.includes(member.content_id ?? -1)}
                          disabled={isTerminal}
                          onCheckedChange={(checked) =>
                            setSplitIds((current) =>
                              checked
                                ? [...current, member.content_id!]
                                : current.filter(
                                    (id) => id !== member.content_id
                                  )
                            )
                          }
                        />
                        移入新事件
                      </label>
                    ) : null}
                  </div>
                ))}
              </div>
            </ScrollArea>
          ) : (
            <p className="text-sm text-muted-foreground">暂无有效聚类成员。</p>
          )}
        </div>
      )}

      {operationError ? (
        <p role="alert" className="mt-3 text-xs text-destructive">
          {operationError}
        </p>
      ) : null}

      {isAdmin ? (
        <Tabs defaultValue="lifecycle" className="mt-5">
          <TabsList
            aria-label="治理操作"
            className={
              members.length > 1
                ? "grid w-full grid-cols-3"
                : "grid w-full grid-cols-2"
            }
          >
            <TabsTrigger value="lifecycle">生命周期</TabsTrigger>
            <TabsTrigger value="merge">合并事件</TabsTrigger>
            {members.length > 1 ? (
              <TabsTrigger value="split">拆分事件</TabsTrigger>
            ) : null}
          </TabsList>

          <TabsContent value="lifecycle" className="mt-4">
            <p className="mb-2 text-xs text-muted-foreground">
              将事件推进到下一治理状态。
            </p>
            <div className="flex gap-2">
              <Select
                value={lifecycle}
                onValueChange={setLifecycle}
                disabled={isTerminal}
              >
                <SelectTrigger
                  aria-label="目标生命周期"
                  className="min-w-0 flex-1"
                >
                  <SelectValue placeholder="选择状态" />
                </SelectTrigger>
                <SelectContent>
                  {(transitions[event.lifecycle_status ?? ""] ?? []).map(
                    (status) => (
                      <SelectItem key={status} value={status}>
                        {lifecycleLabels[status]}
                      </SelectItem>
                    )
                  )}
                </SelectContent>
              </Select>
              <Button
                disabled={busy || !lifecycle || !event.version}
                onClick={() => void onLifecycle(lifecycle)}
              >
                应用
              </Button>
            </div>
          </TabsContent>

          <TabsContent value="merge" className="mt-4">
            <p className="mb-2 text-xs text-muted-foreground">
              将当前事件及其成员合并到已有事件。
            </p>
            <div className="flex gap-2">
              <Select
                value={targetId}
                onValueChange={setTargetId}
                disabled={isTerminal}
              >
                <SelectTrigger aria-label="合并目标" className="min-w-0 flex-1">
                  <SelectValue placeholder="选择目标" />
                </SelectTrigger>
                <SelectContent>
                  {mergeTargets.map((target) => (
                    <SelectItem
                      key={target.event_id}
                      value={String(target.event_id)}
                    >
                      {getRadarEventTitle(target)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button
                    variant="outline"
                    disabled={
                      busy || isTerminal || !selectedTarget || !event.version
                    }
                  >
                    合并
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>确认合并事件？</AlertDialogTitle>
                    <AlertDialogDescription>
                      当前事件将进入“已合并”，聚类成员会转移到目标事件。此操作会写入审计记录。
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>取消</AlertDialogCancel>
                    <AlertDialogAction
                      onClick={() =>
                        selectedTarget && void onMerge(selectedTarget)
                      }
                    >
                      确认合并
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </div>
          </TabsContent>

          {members.length > 1 ? (
            <TabsContent value="split" className="mt-4">
              <div className="flex items-center justify-between gap-3">
                <p className="text-xs text-muted-foreground">
                  已选择 {selectedMembers.length} 条，需保留至少一条。
                </p>
                <AlertDialog>
                  <AlertDialogTrigger asChild>
                    <Button
                      variant="outline"
                      disabled={
                        busy || isTerminal || !splitAllowed || !event.version
                      }
                    >
                      拆分
                    </Button>
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>确认拆分为新事件？</AlertDialogTitle>
                      <AlertDialogDescription>
                        所选成员会进入一个新的待确认事件，原事件保留其余成员。
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>取消</AlertDialogCancel>
                      <AlertDialogAction
                        onClick={() => void onSplit(selectedMembers)}
                      >
                        确认拆分
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </div>
            </TabsContent>
          ) : null}
        </Tabs>
      ) : (
        <p className="mt-4 flex gap-2 text-xs leading-5 text-muted-foreground">
          <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          {canLock
            ? "编辑者可锁定成员；生命周期、合并与拆分仅管理员可操作。"
            : "当前为只读视图；治理操作仅向编辑者和管理员开放。"}
        </p>
      )}
    </section>
  );
}
