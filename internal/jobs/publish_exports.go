package jobs

import (
	"context"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/export"
	"github.com/StephenQiu30/hotkey-server/internal/obsidian"
)

// PublishExportsJob handles periodic export generation and vault publication.
// It orchestrates export bundle creation, report rendering, and atomic vault writes.
type PublishExportsJob struct {
	vaultRoot string
}

// NewPublishExportsJob creates a new PublishExportsJob.
func NewPublishExportsJob(vaultRoot string) *PublishExportsJob {
	return &PublishExportsJob{vaultRoot: vaultRoot}
}

// Name returns the job name for scheduling.
func (j *PublishExportsJob) Name() string { return "publish_exports" }

// Run generates and publishes an export of the given kind.
func (j *PublishExportsJob) Run(ctx context.Context, kind export.ExportKind, input export.BuildExportBundleInput) error {
	// Build the export bundle
	bundle := export.BuildExportBundle(input)

	// Determine the path kind for BuildKnowledgePath
	var pathKind string
	switch kind {
	case export.ExportDaily:
		pathKind = "daily-export"
	case export.ExportWeekly:
		pathKind = "weekly-export"
	case export.ExportMonthly:
		pathKind = "monthly-export"
	case export.ExportThematic:
		pathKind = "thematic-export"
	case export.ExportMaterial:
		pathKind = "material-export"
	default:
		return fmt.Errorf("publish_exports: unknown export kind %q", kind)
	}

	// Build the vault path
	path := obsidian.BuildKnowledgePath(j.vaultRoot, obsidian.PathInput{
		Kind:        pathKind,
		MonitorSlug: fmt.Sprintf("monitor-%d", input.MonitorID),
		Date:        input.DateRange.Start,
		StableID:    fmt.Sprintf("%s-%d", kind, input.MonitorID),
	})

	// For periodic reports, render and write
	var content string
	switch kind {
	case export.ExportDaily, export.ExportWeekly, export.ExportMonthly:
		prInput := export.PeriodicReportInput{
			Title:       fmt.Sprintf("%s Report", kind),
			PeriodLabel: input.DateRange.Start,
		}
		content = export.RenderPeriodicReport(bundle, prInput)
	case export.ExportThematic:
		content = export.RenderThematicReport(bundle, export.ThematicReportInput{
			Title: "专题报告",
		})
	case export.ExportMaterial:
		content = export.RenderMaterialList(export.MaterialListInput{
			ThemeTitle: "素材清单",
		})
	}

	// Write atomically to vault
	return obsidian.WriteAtomic(path, content)
}
