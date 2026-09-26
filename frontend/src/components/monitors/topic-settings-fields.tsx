"use client";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
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
    <Card className="bg-muted gap-6 rounded-2xl py-5 sm:py-7">
      <CardHeader className="px-5 sm:px-7">
        <CardTitle asChild>
          <h2>运行设置</h2>
        </CardTitle>
        <CardDescription className="leading-6">
          只有已应用预设且支持搜索的来源可被保存。新建主题仍保持暂停。
        </CardDescription>
      </CardHeader>

      <CardContent className="flex flex-col gap-6 px-5 sm:px-7">
        <FieldSet disabled={disabled}>
          <FieldLegend variant="label">采集来源</FieldLegend>
          {sourceOptions.length > 0 ? (
            <FieldGroup className="grid gap-2 sm:grid-cols-2">
              {sourceOptions.map((source) => (
                <Field
                  key={source.sourceKey}
                  orientation="horizontal"
                  data-disabled={disabled}
                  className="bg-background rounded-lg px-3 py-2.5"
                >
                  <Checkbox
                    id={`source-${source.sourceKey}`}
                    checked={sourceKeys.includes(source.sourceKey)}
                    onCheckedChange={(checked) =>
                      toggleSource(source.sourceKey, checked === true)
                    }
                    disabled={disabled}
                  />
                  <FieldLabel
                    htmlFor={`source-${source.sourceKey}`}
                    className="font-normal"
                  >
                    {source.displayName}
                    <span className="text-muted-foreground ml-1 font-mono text-xs">
                      {source.sourceKey}
                    </span>
                  </FieldLabel>
                </Field>
              ))}
            </FieldGroup>
          ) : (
            <FieldDescription>
              尚无可选来源。请先在来源能力页应用支持搜索的来源预设。
            </FieldDescription>
          )}
        </FieldSet>

        <FieldGroup className="grid gap-5 sm:grid-cols-2">
          <Field data-disabled={disabled}>
            <FieldLabel htmlFor="collection-interval">
              采集频率（秒）
            </FieldLabel>
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
            <FieldDescription>
              允许 600—86400 秒，默认 1800 秒。
            </FieldDescription>
          </Field>
          <Field data-disabled={disabled}>
            <FieldLabel htmlFor="report-time">每日报告时间</FieldLabel>
            <Input
              id="report-time"
              type="time"
              value={reportTime}
              onChange={(event) => onReportTimeChange(event.target.value)}
              disabled={disabled}
              required
            />
            <FieldDescription>固定使用 Asia/Shanghai 时区。</FieldDescription>
          </Field>
        </FieldGroup>

        <Field
          orientation="horizontal"
          data-disabled={disabled}
          className="justify-between"
        >
          <FieldContent>
            <FieldLabel htmlFor="weekly-report">生成周报</FieldLabel>
            <FieldDescription>
              仅保存偏好；周报流水线将在后续任务实现。
            </FieldDescription>
          </FieldContent>
          <Switch
            id="weekly-report"
            checked={weeklyReportEnabled}
            onCheckedChange={onWeeklyReportEnabledChange}
            disabled={disabled}
            aria-label="生成周报"
          />
        </Field>

        <Field data-disabled={disabled}>
          <FieldLabel htmlFor="notification-targets">推送目标名称</FieldLabel>
          <Textarea
            id="notification-targets"
            value={notificationTargets}
            onChange={(event) =>
              onNotificationTargetsChange(event.target.value)
            }
            disabled={disabled}
            maxLength={2579}
            placeholder={"飞书舆情群\n市场日报邮箱"}
          />
          <FieldDescription>
            每行一个名称，最多 20 个；本阶段暂不校验目标是否已经配置。
          </FieldDescription>
        </Field>
      </CardContent>
    </Card>
  );
}
