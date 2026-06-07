package report

import (
	"fmt"
	"strings"
)

func BuildDailyReportPrompt(date string, hotspots []HotspotData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "你是 HotKey 的中文日报编辑。只输出中文日报，不要输出英文。\n")
	fmt.Fprintf(&b, "日期：%s\n", date)
	fmt.Fprintf(&b, "Prompt Version：%s\n\n", PromptVersion)
	b.WriteString("必须包含：热点摘要、时间线、影响分析、创作者选题建议、来源引用。\n")
	b.WriteString("只能基于给定热点、排序和来源证据写作；不要使用 AI 重新排序或聚类。\n")
	b.WriteString("证据不足时明确写“证据不足，暂不生成影响分析”，不要编造影响分析。\n\n")
	for i, hotspot := range hotspots {
		fmt.Fprintf(&b, "热点 %d：%s\n", i+1, hotspot.Cluster.Title)
		fmt.Fprintf(&b, "排序分数：%.4f\n", hotspot.Score.TotalScore)
		if len(hotspot.Cluster.Keywords) > 0 {
			fmt.Fprintf(&b, "关键词：%s\n", strings.Join(hotspot.Cluster.Keywords, "、"))
		}
		for _, item := range hotspot.Items {
			fmt.Fprintf(&b, "- 来源[%s/%s] %s：%s（%s）\n", item.SourceID, item.ID, item.Title, item.Snippet, item.URL)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func BuildSummaryPrompt(hotspot HotspotData) string {
	return BuildDailyReportPrompt("单热点摘要", []HotspotData{hotspot})
}

func buildDegradedReportBody(date string, hotspots []HotspotData, refs []SourceRef, status ReportStatus) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s 中文热点日报\n\n", date)
	b.WriteString("## 热点摘要\n")
	if len(hotspots) == 0 {
		b.WriteString("暂无足够热点证据。\n")
	} else {
		for _, hotspot := range hotspots {
			fmt.Fprintf(&b, "- %s\n", hotspot.Cluster.Title)
		}
	}
	b.WriteString("\n## 时间线\n")
	b.WriteString("按已有来源时间整理，未补充外部信息。\n\n")
	b.WriteString("## 影响分析\n")
	if status == ReportStatusFailedConfig {
		b.WriteString("DashScope 配置缺失，暂不生成影响分析。\n\n")
	} else {
		b.WriteString("证据不足，暂不生成影响分析。\n\n")
	}
	b.WriteString("## 创作者选题建议\n")
	b.WriteString("优先核验来源引用，再决定是否延展选题。\n\n")
	b.WriteString("## 来源引用\n")
	b.WriteString(formatRefs(refs))
	return b.String()
}

func BuildWeeklyReportPrompt(weekOf string, dailyReports []DailyReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "你是 HotKey 的中文周报编辑。只输出中文周报，不要输出英文。\n")
	fmt.Fprintf(&b, "周期：%s\n", weekOf)
	fmt.Fprintf(&b, "Prompt Version：%s\n\n", PromptVersion)
	b.WriteString("基于以下日报数据，生成本周热点周报。\n")
	b.WriteString("必须包含：本周热点总结、关键事件时间线、趋势分析、创作者选题建议、来源引用。\n")
	b.WriteString("只能基于给定日报内容写作，不要编造信息。\n\n")
	for i, dr := range dailyReports {
		fmt.Fprintf(&b, "日报 %d（%s）：\n%s\n\n", i+1, dr.Date, dr.Body)
	}
	return b.String()
}

func buildDegradedWeeklyReportBody(weekOf string, dailyReports []DailyReport, refs []SourceRef, status ReportStatus) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s 中文热点周报\n\n", weekOf)
	b.WriteString("## 本周热点总结\n")
	if len(dailyReports) == 0 {
		b.WriteString("本周无日报数据。\n")
	} else {
		for _, dr := range dailyReports {
			fmt.Fprintf(&b, "- %s\n", dr.Date)
		}
	}
	b.WriteString("\n## 趋势分析\n")
	if status == ReportStatusFailedConfig {
		b.WriteString("DashScope 配置缺失，暂不生成趋势分析。\n\n")
	} else {
		b.WriteString("证据不足，暂不生成趋势分析。\n\n")
	}
	b.WriteString("## 来源引用\n")
	b.WriteString(formatRefs(refs))
	return b.String()
}

func formatRefs(refs []SourceRef) string {
	if len(refs) == 0 {
		return "- 暂无来源引用\n"
	}
	var b strings.Builder
	for i, ref := range refs {
		fmt.Fprintf(&b, "- [%d] %s（source=%s item=%s）%s\n", i+1, ref.Title, ref.SourceID, ref.ItemID, ref.URL)
	}
	return b.String()
}
