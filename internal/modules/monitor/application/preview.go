package application

import (
	"context"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
)

type PreviewSource struct {
	SourceConnectionID int64
	QuerySignature     string
	IncludedRuleIDs    []int64
	ExcludedRuleIDs    []int64
	UnapprovedRuleIDs  []int64
	EstimatedRequests  int
}

type PreviewResult struct {
	Eligible   bool
	ConfigHash string
	Sources    []PreviewSource
	Warnings   []string
}

// Preview is deliberately read-only: it opens no transaction, never calls an
// external client, and invokes neither AuditWriter nor any repository write.
func (service *Service) Preview(ctx context.Context, subject identitydomain.Subject, monitorID int64) (PreviewResult, error) {
	if err := requireEditor(subject); err != nil {
		return PreviewResult{}, err
	}
	if monitorID <= 0 {
		return PreviewResult{}, domain.MonitorDraftUnavailable()
	}
	monitor, err := service.monitors.FindByID(ctx, monitorID)
	if err != nil {
		return PreviewResult{}, monitorReadError(err)
	}
	if monitor.Status == domain.MonitorStatusArchived || monitor.DraftConfigVersionID == nil {
		return PreviewResult{}, domain.MonitorDraftUnavailable()
	}
	draft, rules, sources, err := service.monitors.FindConfig(ctx, *monitor.DraftConfigVersionID)
	if err != nil {
		return PreviewResult{}, monitorReadError(err)
	}
	if draft.State != domain.ConfigVersionDraft {
		return PreviewResult{}, domain.MonitorDraftUnavailable()
	}
	effective, err := effectiveLocales(draft.Config, rules)
	if err != nil {
		return PreviewResult{}, domain.InvalidMonitorConfiguration()
	}
	hash, err := domain.CanonicalConfigHash(domain.ConfigHashInput{MonitorID: monitor.ID, Revision: draft.Revision, Config: effective, Rules: rules, Sources: sources})
	if err != nil {
		return PreviewResult{}, domain.InvalidMonitorConfiguration()
	}
	result := PreviewResult{Eligible: domain.HasApprovedHumanCoreRule(rules), ConfigHash: hash, Sources: make([]PreviewSource, 0, len(sources)), Warnings: []string{}}
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		connection, err := service.sources.FindForMonitor(ctx, source.SourceConnectionID)
		if err != nil {
			return PreviewResult{}, monitorSourceError(err)
		}
		preview := PreviewSource{SourceConnectionID: source.SourceConnectionID, IncludedRuleIDs: []int64{}, ExcludedRuleIDs: []int64{}, UnapprovedRuleIDs: []int64{}, EstimatedRequests: connection.Config.MaxPagesPerRun}
		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			if rule.ApprovalStatus != domain.RuleApprovalApproved {
				preview.UnapprovedRuleIDs = append(preview.UnapprovedRuleIDs, rule.ID)
				continue
			}
			if rule.RuleType == domain.RuleTypeExcludeKeyword || rule.Operator == domain.RuleOperatorNotEquals {
				preview.ExcludedRuleIDs = append(preview.ExcludedRuleIDs, rule.ID)
			} else {
				preview.IncludedRuleIDs = append(preview.IncludedRuleIDs, rule.ID)
			}
		}
		if !connection.Enabled || connection.Deleted {
			result.Eligible = false
			result.Warnings = append(result.Warnings, "source_connection_unavailable")
			result.Sources = append(result.Sources, preview)
			continue
		}
		if _, err := intersectSourceLocales(effective, connection.Config); err != nil {
			result.Eligible = false
			result.Warnings = append(result.Warnings, "source_locale_intersection_empty")
			result.Sources = append(result.Sources, preview)
			continue
		}
		signature, err := querySignature(source, connection, effective, rules)
		if err != nil {
			return PreviewResult{}, domain.InvalidMonitorConfiguration()
		}
		preview.QuerySignature = signature
		result.Sources = append(result.Sources, preview)
	}
	if len(result.Sources) == 0 {
		result.Eligible = false
		result.Warnings = append(result.Warnings, "no_enabled_sources")
	}
	return result, nil
}
