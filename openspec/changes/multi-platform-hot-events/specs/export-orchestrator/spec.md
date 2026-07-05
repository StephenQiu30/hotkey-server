## MODIFIED Requirements

### Requirement: ExportBundle 结构
The `ExportBundle` struct SHALL support HotEvent as an additional export source alongside Topic.

#### Scenario: 日报包含热点事件 (modified)
- **WHEN** the exporter runs for a daily digest
- **THEN** `ExportBundle.EventIDs` SHALL be populated with HotEvent IDs from the target date range
- **AND** the bundle SHALL include both TopicIDs and EventIDs when both are available

### Requirement: 周期报告渲染 (modified)
The periodic report SHALL support rendering HotEvent data alongside Topic data.

#### Scenario: 日报中的热点事件板块
- **WHEN** `PeriodicReportInput` contains `EventCount > 0`
- **THEN** the rendered report SHALL include a `## 跨平台热点事件` section listing HotEvents with their HeatScore, Platform, and Trend
- **AND** the frontmatter SHALL include `event_count` field

### Requirement: 导出路径覆盖 (modified)
HotEvent SHALL be supported as a standalone exportable entity.

#### Scenario: HotEvent standalone export
- **WHEN** exporting a HotEvent
- **THEN** the file SHALL be written to `{root}/HotKey/events/{slug}/{date}-{id}-{title}.md` (consistent with existing Event path)
- **AND** the frontmatter SHALL include `type: hotkey-event`, `event_id`, `platform`, `heat_score`, `related_topics`
