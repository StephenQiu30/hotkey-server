"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";

export type TopicSourceOption = {
  sourceKey: string;
  displayName: string;
};

export function selectableTopicSources(
  platforms: HotKeyAPI.SourcePlatformView[],
  selectedSourceKeys: string[] = [],
): TopicSourceOption[] {
  const available = platforms
    .filter(
      (platform) =>
        platform.connection_status === "active" &&
        !platform.has_credentials &&
        platform.capabilities.some(
          (capability) => capability.capability === "search",
        ),
    )
    .map((platform) => ({
      sourceKey: platform.source_key,
      displayName: platform.display_name,
    }));
  const availableKeys = new Set(available.map((source) => source.sourceKey));
  const unavailableSelections = selectedSourceKeys
    .filter((sourceKey) => !availableKeys.has(sourceKey))
    .map((sourceKey) => ({
      sourceKey,
      displayName: `${sourceKey}（当前不可用）`,
    }));
  return [...available, ...unavailableSelections];
}

type TopicSettingsFieldsProps = {
  sourceOptions: TopicSourceOption[];
  sourceKeys: string[];
  onSourceKeysChange: (value: string[]) => void;
  collectionIntervalSeconds: number;
  onCollectionIntervalSecondsChange: (value: number) => void;
  reportTime: string;
  onReportTimeChange: (value: string) => void;
  weeklyReportEnabled: boolean;
  onWeeklyReportEnabledChange: (value: boolean) => void;
  notificationTargets: string;
  onNotificationTargetsChange: (value: string) => void;
  disabled: boolean;
};

export function parseNotificationTargetNames(value: string): string[] {
  return Array.from(
    new Set(
      value
        .split("\n")
        .map((item) => item.trim())
        .filter(Boolean),
    ),
  );
}

export function TopicSettingsFields({
  sourceOptions,
  sourceKeys,
  onSourceKeysChange,
  collectionIntervalSeconds,
  onCollectionIntervalSecondsChange,
  reportTime,
  onReportTimeChange,
  weeklyReportEnabled,
  onWeeklyReportEnabledChange,
  notificationTargets,
  onNotificationTargetsChange,
  disabled,
}: TopicSettingsFieldsProps) {
  function toggleSource(sourceKey: string, selected: boolean) {
    onSourceKeysChange(
      selected
        ? Array.from(new Set([...sourceKeys, sourceKey])).sort()
        : sourceKeys.filter((item) => item !== sourceKey),
    );
  }

  return (
    <section className="bg-muted space-y-6 rounded-2xl p-5 sm:p-7">
      <div>
        <h2 className="text-base font-medium">运行设置</h2>
        <p className="text-muted-foreground mt-1 text-sm leading-6">
          只有已应用预设且支持搜索的来源可被保存。新建主题仍保持暂停。
        </p>
      </div>

      <fieldset className="space-y-3" disabled={disabled}>
        <legend className="text-sm font-medium">采集来源</legend>
        {sourceOptions.length > 0 ? (
          <div className="grid gap-2 sm:grid-cols-2">
            {sourceOptions.map((source) => (
              <label
                key={source.sourceKey}
                className="bg-background flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2.5 text-sm"
              >
                <input
                  type="checkbox"
                  className="border-input size-4 rounded"
                  checked={sourceKeys.includes(source.sourceKey)}
                  onChange={(event) =>
                    toggleSource(source.sourceKey, event.target.checked)
                  }
                />
                <span>
                  {source.displayName}
                  <span className="text-muted-foreground ml-1 font-mono text-xs">
                    {source.sourceKey}
                  </span>
                </span>
              </label>
            ))}
          </div>
        ) : (
          <p className="text-muted-foreground text-sm leading-6">
            尚无可选来源。请先在来源能力页应用支持搜索的来源预设。
          </p>
        )}
      </fieldset>

      <div className="grid gap-5 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="collection-interval">采集频率（秒）</Label>
          <Input
            id="collection-interval"
            type="number"
            min={600}
            max={86400}
            step={60}
            value={collectionIntervalSeconds}
            onChange={(event) =>
              onCollectionIntervalSecondsChange(event.target.valueAsNumber)
            }
            disabled={disabled}
            required
          />
          <p className="text-muted-foreground text-xs leading-5">
            允许 600—86400 秒，默认 1800 秒。
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="report-time">每日报告时间</Label>
          <Input
            id="report-time"
            type="time"
            value={reportTime}
            onChange={(event) => onReportTimeChange(event.target.value)}
            disabled={disabled}
            required
          />
          <p className="text-muted-foreground text-xs leading-5">
            固定使用 Asia/Shanghai 时区。
          </p>
        </div>
      </div>

      <div className="flex items-center justify-between gap-4">
        <div>
          <Label htmlFor="weekly-report">生成周报</Label>
          <p className="text-muted-foreground mt-1 text-xs leading-5">
            仅保存偏好；周报流水线将在后续任务实现。
          </p>
        </div>
        <Switch
          id="weekly-report"
          checked={weeklyReportEnabled}
          onCheckedChange={onWeeklyReportEnabledChange}
          disabled={disabled}
          aria-label="生成周报"
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="notification-targets">推送目标名称</Label>
        <Textarea
          id="notification-targets"
          value={notificationTargets}
          onChange={(event) => onNotificationTargetsChange(event.target.value)}
          disabled={disabled}
          maxLength={2579}
          placeholder={"飞书舆情群\n市场日报邮箱"}
        />
        <p className="text-muted-foreground text-xs leading-5">
          每行一个名称，最多 20 个；本阶段暂不校验目标是否已经配置。
        </p>
      </div>
    </section>
  );
}
