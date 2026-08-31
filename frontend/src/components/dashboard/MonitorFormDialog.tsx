import { type FormEvent } from "react";
import { Loader2, RotateCcw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Surface } from "@/components/ui/surface";
import { sourceTypeLabel } from "@/lib/sourceLabels";
import { type SimpleMonitorForm } from "@/lib/monitorWorkflow";

type MonitorFormDialogProps = {
  open: boolean;
  editTarget?: HotKeyAPI.MonitorResponse;
  form: SimpleMonitorForm;
  sources: HotKeyAPI.SourceReadResponse[];
  saving: boolean;
  onOpenChange: (open: boolean) => void;
  onFormChange: (form: SimpleMonitorForm) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
};

export function MonitorFormDialog({
  open,
  editTarget,
  form,
  sources,
  saving,
  onOpenChange,
  onFormChange,
  onSubmit,
}: MonitorFormDialogProps) {
  function update(fields: Partial<SimpleMonitorForm>) {
    onFormChange({ ...form, ...fields });
  }

  function toggleSource(sourceID: number) {
    update({
      sourceIds: form.sourceIds.includes(sourceID)
        ? form.sourceIds.filter((value) => value !== sourceID)
        : [...form.sourceIds, sourceID].slice(0, 10),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {editTarget ? "编辑监控任务" : "新建监控任务"}
          </DialogTitle>
          <DialogDescription>
            {editTarget
              ? "修改简单字段后立即生效；暂停中的监控仍保持暂停。"
              : "只需填写监控词和来源；创建后立即启用。"}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-5" onSubmit={onSubmit}>
          <div>
            <Label htmlFor="monitor-name">监控名称</Label>
            <Input
              id="monitor-name"
              className="mt-2"
              value={form.name}
              maxLength={120}
              onChange={(event) => update({ name: event.target.value })}
            />
          </div>
          <div>
            <Label htmlFor="monitor-query">监控词</Label>
            <Input
              id="monitor-query"
              className="mt-2"
              value={form.query}
              maxLength={160}
              placeholder="例如 Claude、OpenAI、具身智能"
              onChange={(event) => update({ query: event.target.value })}
            />
          </div>
          <div>
            <Label htmlFor="monitor-interval">扫描间隔</Label>
            <Select value={form.interval} onValueChange={(interval) => update({ interval })}>
              <SelectTrigger
                id="monitor-interval"
                aria-label="扫描间隔"
                className="mt-2"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="900">15 分钟</SelectItem>
                <SelectItem value="1800">30 分钟</SelectItem>
                <SelectItem value="3600">1 小时</SelectItem>
                <SelectItem value="21600">6 小时</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <fieldset>
            <legend className="text-sm font-medium">来源</legend>
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              {sources.map((source) =>
                source.id == null ? null : (
                  <label key={source.id} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={form.sourceIds.includes(source.id)}
                      onCheckedChange={() => toggleSource(source.id as number)}
                    />
                    {source.name || sourceTypeLabel(source.source_type)}
                  </label>
                )
              )}
            </div>
          </fieldset>
          <Surface asChild variant="ring">
            <label className="flex items-start gap-3 p-3">
              <Checkbox
                aria-label="高优先级邮件提醒"
                checked={form.alertEmailEnabled}
                onCheckedChange={(checked) =>
                  update({ alertEmailEnabled: checked === true })
                }
              />
              <span>
                <span className="block text-sm font-medium">高优先级邮件提醒</span>
                <span className="mt-1 block text-xs text-muted-foreground">
                  仅在事件达到高或紧急热度时发送到当前账号邮箱。
                </span>
              </span>
            </label>
          </Surface>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={saving || form.sourceIds.length === 0}>
              {saving ? <Loader2 className="animate-spin" /> : <RotateCcw />}
              {editTarget ? "保存修改" : "创建并启用"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
