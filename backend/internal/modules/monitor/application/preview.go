package application

import (
	"context"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/monitor/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

type PreviewSource struct {
	SourceConnectionID int64
	QuerySignature     string
	CompiledQuery      string
	QueryMode          string
	Languages          []string
	Regions            []string
	MaxQueryBytes      int
	IncludedRuleIDs    []int64
	ExcludedRuleIDs    []int64
	UnapprovedRuleIDs  []int64
	IncludedTermCount  int
	ExcludedTermCount  int
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
	if err := requireContributor(subject); err != nil {
		return PreviewResult{}, err
	}
	if monitorID <= 0 {
		return PreviewResult{}, domain.MonitorDraftUnavailable()
	}
	monitor, err := service.monitors.FindByID(ctx, monitorID)
	if err != nil {
		return PreviewResult{}, monitorReadError(err)
	}
	if err := authorizeMonitorContributor(subject, *monitor); err != nil {
		return PreviewResult{}, err
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
	intentPreview := PreviewIntentPublicationResult{}
	if service.intentPublication != nil {
		intentPreview, err = service.intentPublication.Preview(ctx, PreviewIntentPublicationCommand{
			MonitorID: monitor.ID, ConfigVersionID: draft.ID,
		})
		if err != nil {
			return PreviewResult{}, monitorIntentPublicationError(err)
		}
	}
	var effective domain.MonitorConfig
	if intentPreview.Enabled {
		effective, err = effectiveIntentLocales(draft.Config, intentPreview.LocaleClauses)
	} else {
		effective, err = effectiveLocales(draft.Config, rules)
	}
	if err != nil {
		return PreviewResult{}, domain.InvalidMonitorConfiguration()
	}
	hash, err := domain.CanonicalConfigHash(domain.ConfigHashInput{MonitorID: monitor.ID, Revision: draft.Revision, Config: effective, Rules: rules, Sources: sources})
	if err != nil {
		return PreviewResult{}, domain.InvalidMonitorConfiguration()
	}
	if intentPreview.Enabled {
		hash = intentPublishedConfigHash(hash, intentPreview.ProfileHash)
	}
	result := PreviewResult{Eligible: intentPreview.Enabled || domain.HasApprovedHumanCoreRule(rules), ConfigHash: hash, Sources: make([]PreviewSource, 0, len(sources)), Warnings: []string{}}
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		connection, err := service.sources.FindForMonitor(ctx, source.SourceConnectionID)
		if err != nil {
			return PreviewResult{}, monitorSourceError(err)
		}
		preview := PreviewSource{SourceConnectionID: source.SourceConnectionID, QueryMode: "local_filter", MaxQueryBytes: sourcedomain.MaxCollectionQueryBytes, IncludedRuleIDs: []int64{}, ExcludedRuleIDs: []int64{}, UnapprovedRuleIDs: []int64{}, EstimatedRequests: connection.Config.MaxPagesPerRun}
		for _, rule := range rules {
			if intentPreview.Enabled {
				break
			}
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
		if intentPreview.Enabled {
			for _, term := range intentPreview.CollectionTerms {
				if term.Excluded {
					preview.ExcludedTermCount++
				} else {
					preview.IncludedTermCount++
				}
			}
		} else {
			preview.IncludedTermCount = len(preview.IncludedRuleIDs)
			preview.ExcludedTermCount = len(preview.ExcludedRuleIDs)
		}
		if !connection.Enabled || connection.Deleted {
			result.Eligible = false
			result.Warnings = append(result.Warnings, "source_connection_unavailable")
			result.Sources = append(result.Sources, preview)
			continue
		}
		perSource, err := intersectSourceLocales(effective, connection.Config)
		if err != nil {
			result.Eligible = false
			result.Warnings = append(result.Warnings, "source_locale_intersection_empty")
			result.Sources = append(result.Sources, preview)
			continue
		}
		preview.Languages = append([]string(nil), perSource.Languages...)
		preview.Regions = append([]string(nil), perSource.Regions...)
		terms := collectionTerms(rules)
		if intentPreview.Enabled {
			terms = compiledCollectionTerms(intentPreview.CollectionTerms)
		}
		preview.CompiledQuery, err = sourcedomain.CompileCollectionQuery(source.QueryOverride, terms)
		if err != nil {
			result.Eligible = false
			result.Warnings = append(result.Warnings, "source_query_limit_exceeded")
			result.Sources = append(result.Sources, preview)
			continue
		}
		var signature string
		if intentPreview.Enabled {
			signature, err = querySignatureFromCompiledIntent(source, connection, effective, intentPreview.CollectionTerms, intentPreview.ProfileHash)
		} else {
			signature, err = querySignature(source, connection, effective, rules)
		}
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

func compiledCollectionTerms(values []CompiledCollectionTermDTO) []sourcedomain.CollectionTerm {
	terms := make([]sourcedomain.CollectionTerm, len(values))
	for index, value := range values {
		terms[index] = sourcedomain.CollectionTerm{Value: value.Value, Excluded: value.Excluded}
	}
	return terms
}

func collectionTerms(rules []domain.MonitorRule) []sourcedomain.CollectionTerm {
	terms := make([]sourcedomain.CollectionTerm, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled || rule.ApprovalStatus != domain.RuleApprovalApproved {
			continue
		}
		switch rule.RuleType {
		case domain.RuleTypeKeyword, domain.RuleTypePhrase, domain.RuleTypeEntity:
			terms = append(terms, sourcedomain.CollectionTerm{Value: rule.Value})
		case domain.RuleTypeExcludeKeyword:
			terms = append(terms, sourcedomain.CollectionTerm{Value: rule.Value, Excluded: true})
		}
	}
	return terms
}
