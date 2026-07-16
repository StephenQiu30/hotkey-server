// Package application coordinates Monitor configuration commands. It owns
// authorization, optimistic versions and transactions; it never reaches into
// SourceConnection persistence directly.
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/monitor/domain"
	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	sourcedomain "github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/internal/shared/requestcontext"
)

const configurationAdvisoryLock = "hotkey.monitor_source_configuration"

type Dependencies struct {
	Runtime  *database.Runtime
	Monitors domain.MonitorRepository
	Sources  sourcedomain.MonitorSourceReader
	Audit    operationsapplication.AuditWriter
}

type Service struct {
	runtime  *database.Runtime
	monitors domain.MonitorRepository
	sources  sourcedomain.MonitorSourceReader
	audit    operationsapplication.AuditWriter
}

func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Runtime == nil || dependencies.Monitors == nil || dependencies.Sources == nil || dependencies.Audit == nil {
		return nil, errors.New("monitor application dependencies are required")
	}
	return &Service{runtime: dependencies.Runtime, monitors: dependencies.Monitors, sources: dependencies.Sources, audit: dependencies.Audit}, nil
}

type DraftInput struct {
	Name        string
	Description string
	Config      domain.MonitorConfig
	Rules       []domain.MonitorRule
	Sources     []domain.MonitorSource
}

type CreateInput struct {
	Subject identitydomain.Subject
	Draft   DraftInput
}
type ReplaceDraftInput struct {
	Subject   identitydomain.Subject
	MonitorID int64
	Expected  domain.ExpectedVersions
	Draft     DraftInput
}
type AICandidateInput struct {
	Subject   identitydomain.Subject
	MonitorID int64
	Expected  domain.ExpectedVersions
	Rule      domain.MonitorRule
}
type ApprovalInput struct {
	Subject           identitydomain.Subject
	MonitorID, RuleID int64
	Expected          domain.ExpectedVersions
	Approval          domain.RuleApprovalStatus
}
type PublishInput struct {
	Subject   identitydomain.Subject
	MonitorID int64
	Expected  domain.ExpectedVersions
}
type LifecycleInput struct {
	Subject                           identitydomain.Subject
	MonitorID, ExpectedMonitorVersion int64
}

// Create creates exactly one draft configuration. Source facts may be disabled
// in a draft, but their IDs must be real; publish is where schedulability is
// enforced atomically.
func (service *Service) Create(ctx context.Context, input CreateInput) (*domain.Monitor, *domain.MonitorConfigVersion, error) {
	if err := requireEditor(input.Subject); err != nil {
		return nil, nil, err
	}
	draft, err := service.normalizeDraft(ctx, input.Draft, nil, false)
	if err != nil {
		return nil, nil, err
	}
	monitor := domain.Monitor{Name: draft.Name, Description: draft.Description, Status: domain.MonitorStatusDraft}
	config := domain.MonitorConfigVersion{Revision: 1, State: domain.ConfigVersionDraft, Config: draft.Config}
	err = service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockConfiguration(ctx, transaction); err != nil {
			return err
		}
		if err := service.monitors.Create(ctx, &monitor, &config, draft.Rules, draft.Sources); err != nil {
			if errors.Is(err, sharedrepository.ErrConflict) {
				return domain.MonitorNameConflict()
			}
			return monitorWriteError(err)
		}
		if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, operationsdomain.ActionMonitorCreated, monitor, &config, len(draft.Rules), len(draft.Sources), nil)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &monitor, &config, nil
}

func (service *Service) ReplaceDraft(ctx context.Context, input ReplaceDraftInput) (*domain.Monitor, *domain.MonitorConfigVersion, error) {
	if err := requireEditor(input.Subject); err != nil {
		return nil, nil, err
	}
	if input.MonitorID <= 0 {
		return nil, nil, domain.MonitorDraftUnavailable()
	}
	var changed domain.Monitor
	var draftResult domain.MonitorConfigVersion
	err := service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockConfiguration(ctx, transaction); err != nil {
			return err
		}
		monitor, err := service.monitors.LockByID(ctx, input.MonitorID)
		if err != nil {
			return monitorReadError(err)
		}
		if monitor.Status == domain.MonitorStatusArchived {
			return domain.InvalidMonitorState()
		}
		if monitor.Version != input.Expected.MonitorVersion {
			return domain.MonitorVersionConflict()
		}
		if monitor.DraftConfigVersionID != nil {
			if err := input.Expected.ValidateDraft(true); err != nil {
				return domain.MonitorVersionConflict()
			}
			config, rules, _, err := service.monitors.LockConfig(ctx, *monitor.DraftConfigVersionID)
			if err != nil {
				return monitorReadError(err)
			}
			if config.State != domain.ConfigVersionDraft || config.Version != *input.Expected.DraftVersion {
				return domain.MonitorVersionConflict()
			}
			draft, err := service.normalizeDraft(ctx, input.Draft, rules, true)
			if err != nil {
				return err
			}
			config.Version++
			config.Config = draft.Config
			monitor.Name, monitor.Description, monitor.Version = draft.Name, draft.Description, monitor.Version+1
			if err := service.monitors.SaveDraft(ctx, config, draft.Rules, draft.Sources); err != nil {
				return monitorWriteError(err)
			}
			if err := service.monitors.SaveMonitor(ctx, monitor); err != nil {
				if errors.Is(err, sharedrepository.ErrConflict) {
					return domain.MonitorNameConflict()
				}
				return monitorWriteError(err)
			}
			if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, operationsdomain.ActionMonitorDraftUpdated, *monitor, config, len(draft.Rules), len(draft.Sources), nil)); err != nil {
				return err
			}
			changed, draftResult = *monitor, *config
			return nil
		}
		if err := input.Expected.ValidateDraft(false); err != nil {
			return domain.MonitorVersionConflict()
		}
		if monitor.PublishedConfigVersionID == nil {
			return domain.MonitorDraftUnavailable()
		}
		published, _, _, err := service.monitors.LockConfig(ctx, *monitor.PublishedConfigVersionID)
		if err != nil {
			return monitorReadError(err)
		}
		draft, err := service.normalizeDraft(ctx, input.Draft, nil, false)
		if err != nil {
			return err
		}
		config := domain.MonitorConfigVersion{MonitorID: monitor.ID, Revision: published.Revision + 1, State: domain.ConfigVersionDraft, Config: draft.Config}
		if err := service.monitors.CreateDraft(ctx, &config, draft.Rules, draft.Sources); err != nil {
			return monitorWriteError(err)
		}
		monitor.Name, monitor.Description, monitor.DraftConfigVersionID, monitor.Version = draft.Name, draft.Description, int64Pointer(config.ID), monitor.Version+1
		if err := service.monitors.SaveMonitor(ctx, monitor); err != nil {
			if errors.Is(err, sharedrepository.ErrConflict) {
				return domain.MonitorNameConflict()
			}
			return monitorWriteError(err)
		}
		if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, operationsdomain.ActionMonitorDraftUpdated, *monitor, &config, len(draft.Rules), len(draft.Sources), nil)); err != nil {
			return err
		}
		changed, draftResult = *monitor, config
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &changed, &draftResult, nil
}

func (service *Service) AddAICandidate(ctx context.Context, input AICandidateInput) (*domain.MonitorConfigVersion, *domain.MonitorRule, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, nil, err
	}
	input.Rule.ID, input.Rule.Version, input.Rule.ConfigVersionID = 0, 0, 0
	input.Rule.Origin, input.Rule.ApprovalStatus, input.Rule.Enabled = domain.RuleOriginAI, domain.RuleApprovalPending, true
	rule, err := domain.NormalizeRule(input.Rule)
	if err != nil {
		return nil, nil, domain.InvalidMonitorConfiguration()
	}
	var resultConfig domain.MonitorConfigVersion
	var resultRule domain.MonitorRule
	err = service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockConfiguration(ctx, transaction); err != nil {
			return err
		}
		monitor, config, rules, sources, err := service.lockExpectedDraftInTransaction(ctx, input.MonitorID, input.Expected)
		if err != nil {
			return err
		}
		if len(rules) >= 100 {
			return domain.InvalidMonitorConfiguration()
		}
		rules = append(rules, rule)
		config.Version++
		monitor.Version++
		if err := service.monitors.SaveDraft(ctx, config, rules, sources); err != nil {
			return monitorWriteError(err)
		}
		if err := service.monitors.SaveMonitor(ctx, monitor); err != nil {
			return monitorWriteError(err)
		}
		resultRule = rules[len(rules)-1]
		if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, operationsdomain.ActionMonitorAICandidateCreated, *monitor, config, len(rules), len(sources), map[string]any{"approval_status": string(domain.RuleApprovalPending)})); err != nil {
			return err
		}
		resultConfig = *config
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &resultConfig, &resultRule, nil
}

func (service *Service) ApproveAICandidate(ctx context.Context, input ApprovalInput) (*domain.MonitorConfigVersion, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, err
	}
	if input.Approval != domain.RuleApprovalApproved && input.Approval != domain.RuleApprovalRejected {
		return nil, domain.InvalidMonitorConfiguration()
	}
	var result domain.MonitorConfigVersion
	err := service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockConfiguration(ctx, transaction); err != nil {
			return err
		}
		monitor, config, rules, sources, err := service.lockExpectedDraftInTransaction(ctx, input.MonitorID, input.Expected)
		if err != nil {
			return err
		}
		found := false
		for index := range rules {
			if rules[index].ID == input.RuleID && rules[index].Origin == domain.RuleOriginAI && rules[index].ApprovalStatus == domain.RuleApprovalPending {
				rules[index].ApprovalStatus = input.Approval
				found = true
			}
		}
		if !found {
			return domain.InvalidMonitorConfiguration()
		}
		config.Version++
		monitor.Version++
		if err := service.monitors.SaveDraft(ctx, config, rules, sources); err != nil {
			return monitorWriteError(err)
		}
		if err := service.monitors.SaveMonitor(ctx, monitor); err != nil {
			return monitorWriteError(err)
		}
		action := operationsdomain.ActionMonitorAICandidateApproved
		if input.Approval == domain.RuleApprovalRejected {
			action = operationsdomain.ActionMonitorAICandidateRejected
		}
		if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, action, *monitor, config, len(rules), len(sources), map[string]any{"approval_status": string(input.Approval)})); err != nil {
			return err
		}
		result = *config
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (service *Service) Publish(ctx context.Context, input PublishInput) (*domain.Monitor, *domain.MonitorConfigVersion, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, nil, err
	}
	var changed domain.Monitor
	var publishedResult domain.MonitorConfigVersion
	err := service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockConfiguration(ctx, transaction); err != nil {
			return err
		}
		monitor, draft, rules, sources, err := service.lockPublishableDraft(ctx, input.MonitorID, input.Expected)
		if err != nil {
			return err
		}
		if !domain.HasApprovedHumanCoreRule(rules) {
			return domain.InvalidMonitorConfiguration()
		}
		effective, sourceFacts, err := service.validatePublishSources(ctx, draft.Config, rules, sources, true)
		if err != nil {
			return err
		}
		hash, err := domain.CanonicalConfigHash(domain.ConfigHashInput{MonitorID: monitor.ID, Revision: draft.Revision, Config: effective, Rules: rules, Sources: sources})
		if err != nil {
			return domain.InvalidMonitorConfiguration()
		}
		for index := range sources {
			if sources[index].Enabled {
				signature, err := querySignature(sources[index], sourceFacts[sources[index].SourceConnectionID], effective, rules)
				if err != nil {
					return domain.InvalidMonitorConfiguration()
				}
				sources[index].QuerySignature = signature
			}
		}
		var previous *domain.MonitorConfigVersion
		if monitor.PublishedConfigVersionID != nil {
			previous, _, _, err = service.monitors.LockConfig(ctx, *monitor.PublishedConfigVersionID)
			if err != nil {
				return monitorReadError(err)
			}
		}
		now := time.Now().UTC()
		draft.Config, draft.State, draft.ConfigHash, draft.PublishedAt, draft.Version = effective, domain.ConfigVersionPublished, hash, &now, draft.Version+1
		monitor.Status, monitor.DraftConfigVersionID, monitor.PublishedConfigVersionID, monitor.Version = domain.MonitorStatusActive, nil, int64Pointer(draft.ID), monitor.Version+1
		if err := service.monitors.Publish(ctx, monitor, draft, previous, sources); err != nil {
			return monitorWriteError(err)
		}
		if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, operationsdomain.ActionMonitorPublished, *monitor, draft, len(rules), len(sources), nil)); err != nil {
			return err
		}
		changed, publishedResult = *monitor, *draft
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &changed, &publishedResult, nil
}

func (service *Service) Pause(ctx context.Context, input LifecycleInput) (*domain.Monitor, error) {
	return service.changeState(ctx, input, domain.MonitorStatusPaused, operationsdomain.ActionMonitorPaused)
}
func (service *Service) Resume(ctx context.Context, input LifecycleInput) (*domain.Monitor, error) {
	return service.changeState(ctx, input, domain.MonitorStatusActive, operationsdomain.ActionMonitorResumed)
}
func (service *Service) Archive(ctx context.Context, input LifecycleInput) (*domain.Monitor, error) {
	return service.changeState(ctx, input, domain.MonitorStatusArchived, operationsdomain.ActionMonitorArchived)
}
func (service *Service) Restore(ctx context.Context, input LifecycleInput) (*domain.Monitor, error) {
	return service.changeState(ctx, input, domain.MonitorStatusPaused, operationsdomain.ActionMonitorRestored)
}

func (service *Service) changeState(ctx context.Context, input LifecycleInput, target domain.MonitorStatus, action operationsdomain.AuditAction) (*domain.Monitor, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, err
	}
	if input.MonitorID <= 0 || input.ExpectedMonitorVersion <= 0 {
		return nil, domain.MonitorVersionConflict()
	}
	var changed domain.Monitor
	err := service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockConfiguration(ctx, transaction); err != nil {
			return err
		}
		monitor, err := service.monitors.LockByID(ctx, input.MonitorID)
		if err != nil {
			return monitorReadError(err)
		}
		if monitor.Version != input.ExpectedMonitorVersion {
			return domain.MonitorVersionConflict()
		}
		if monitor.Status == target {
			changed = *monitor
			return nil
		}
		if action == operationsdomain.ActionMonitorRestored {
			if monitor.Status != domain.MonitorStatusArchived {
				return domain.InvalidMonitorState()
			}
		} else if monitor.Status == domain.MonitorStatusArchived {
			return domain.InvalidMonitorState()
		} else if !domain.CanTransition(monitor.Status, target) {
			return domain.InvalidMonitorState()
		}
		if target == domain.MonitorStatusActive {
			if monitor.PublishedConfigVersionID == nil {
				return domain.InvalidMonitorState()
			}
			config, rules, sources, err := service.monitors.LockConfig(ctx, *monitor.PublishedConfigVersionID)
			if err != nil {
				return monitorReadError(err)
			}
			if config.State != domain.ConfigVersionPublished {
				return domain.InvalidMonitorState()
			}
			if _, _, err := service.validatePublishSources(ctx, config.Config, rules, sources, true); err != nil {
				return err
			}
		}
		previous := monitor.Status
		monitor.Status, monitor.Version = target, monitor.Version+1
		if err := service.monitors.SaveMonitor(ctx, monitor); err != nil {
			if errors.Is(err, sharedrepository.ErrConflict) || errors.Is(err, sharedrepository.ErrConstraint) {
				return domain.MonitorNameConflict()
			}
			return monitorWriteError(err)
		}
		if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, action, *monitor, nil, 0, 0, map[string]any{"previous_status": string(previous)})); err != nil {
			return err
		}
		changed = *monitor
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &changed, nil
}

func (service *Service) ActivePublished(ctx context.Context, subject identitydomain.Subject) ([]domain.PublishedMonitor, error) {
	if err := requireAuthenticated(subject); err != nil {
		return nil, err
	}
	result, err := service.monitors.ListActivePublished(ctx)
	if err != nil {
		return nil, monitorReadError(err)
	}
	return result, nil
}

func (service *Service) lockExpectedDraftInTransaction(ctx context.Context, id int64, expected domain.ExpectedVersions) (*domain.Monitor, *domain.MonitorConfigVersion, []domain.MonitorRule, []domain.MonitorSource, error) {
	if id <= 0 || expected.ValidateDraft(true) != nil {
		return nil, nil, nil, nil, domain.MonitorVersionConflict()
	}
	monitor, err := service.monitors.LockByID(ctx, id)
	if err != nil {
		return nil, nil, nil, nil, monitorReadError(err)
	}
	if monitor.Version != expected.MonitorVersion {
		return nil, nil, nil, nil, domain.MonitorVersionConflict()
	}
	if monitor.Status == domain.MonitorStatusArchived || monitor.DraftConfigVersionID == nil {
		return nil, nil, nil, nil, domain.MonitorDraftUnavailable()
	}
	config, rules, sources, err := service.monitors.LockConfig(ctx, *monitor.DraftConfigVersionID)
	if err != nil {
		return nil, nil, nil, nil, monitorReadError(err)
	}
	if config.State != domain.ConfigVersionDraft || config.Version != *expected.DraftVersion {
		return nil, nil, nil, nil, domain.MonitorVersionConflict()
	}
	return monitor, config, rules, sources, nil
}

func (service *Service) lockPublishableDraft(ctx context.Context, id int64, expected domain.ExpectedVersions) (*domain.Monitor, *domain.MonitorConfigVersion, []domain.MonitorRule, []domain.MonitorSource, error) {
	if id <= 0 || expected.ValidateDraft(true) != nil {
		return nil, nil, nil, nil, domain.MonitorVersionConflict()
	}
	monitor, err := service.monitors.LockByID(ctx, id)
	if err != nil {
		return nil, nil, nil, nil, monitorReadError(err)
	}
	if monitor.Version != expected.MonitorVersion {
		return nil, nil, nil, nil, domain.MonitorVersionConflict()
	}
	if monitor.DraftConfigVersionID == nil {
		return nil, nil, nil, nil, domain.MonitorDraftUnavailable()
	}
	if monitor.Status != domain.MonitorStatusDraft && monitor.Status != domain.MonitorStatusActive {
		return nil, nil, nil, nil, domain.InvalidMonitorState()
	}
	draft, rules, sources, err := service.monitors.LockConfig(ctx, *monitor.DraftConfigVersionID)
	if err != nil {
		return nil, nil, nil, nil, monitorReadError(err)
	}
	if draft.State != domain.ConfigVersionDraft || draft.Version != *expected.DraftVersion {
		return nil, nil, nil, nil, domain.MonitorVersionConflict()
	}
	return monitor, draft, rules, sources, nil
}

type normalizedDraft struct {
	Name, Description string
	Config            domain.MonitorConfig
	Rules             []domain.MonitorRule
	Sources           []domain.MonitorSource
}

func (service *Service) normalizeDraft(ctx context.Context, input DraftInput, existing []domain.MonitorRule, retainNonUser bool) (normalizedDraft, error) {
	name, err := domain.NormalizeMonitorName(input.Name)
	if err != nil {
		return normalizedDraft{}, domain.InvalidMonitorConfiguration()
	}
	description, err := domain.NormalizeMonitorDescription(input.Description)
	if err != nil {
		return normalizedDraft{}, domain.InvalidMonitorConfiguration()
	}
	config, err := domain.NormalizeMonitorConfig(input.Config)
	if err != nil {
		return normalizedDraft{}, domain.InvalidMonitorConfiguration()
	}
	if len(input.Rules) > 100 || len(input.Sources) > 10 {
		return normalizedDraft{}, domain.InvalidMonitorConfiguration()
	}
	rules := make([]domain.MonitorRule, 0, len(input.Rules)+len(existing))
	for _, candidate := range input.Rules {
		candidate.ID, candidate.Version, candidate.ConfigVersionID = 0, 0, 0
		candidate.Origin, candidate.ApprovalStatus = domain.RuleOriginUser, domain.RuleApprovalApproved
		normalized, err := domain.NormalizeRule(candidate)
		if err != nil {
			return normalizedDraft{}, domain.InvalidMonitorConfiguration()
		}
		rules = append(rules, normalized)
	}
	if retainNonUser {
		for _, candidate := range existing {
			if candidate.Origin != domain.RuleOriginUser {
				rules = append(rules, candidate)
			}
		}
	}
	if len(rules) > 100 {
		return normalizedDraft{}, domain.InvalidMonitorConfiguration()
	}
	sources := make([]domain.MonitorSource, 0, len(input.Sources))
	seenSources := make(map[int64]struct{}, len(input.Sources))
	for _, candidate := range input.Sources {
		candidate.ID, candidate.Version, candidate.ConfigVersionID, candidate.QuerySignature = 0, 0, 0, ""
		if candidate.SourceConnectionID <= 0 {
			return normalizedDraft{}, domain.InvalidMonitorConfiguration()
		}
		if _, exists := seenSources[candidate.SourceConnectionID]; exists {
			return normalizedDraft{}, domain.InvalidMonitorConfiguration()
		}
		seenSources[candidate.SourceConnectionID] = struct{}{}
		override, err := domain.NormalizeQueryOverride(candidate.QueryOverride)
		if err != nil {
			return normalizedDraft{}, domain.InvalidMonitorConfiguration()
		}
		candidate.QueryOverride = override
		sources = append(sources, candidate)
		if _, err := service.sources.FindForMonitor(ctx, candidate.SourceConnectionID); err != nil {
			return normalizedDraft{}, monitorSourceError(err)
		}
	}
	return normalizedDraft{Name: name, Description: description, Config: config, Rules: rules, Sources: sources}, nil
}

func (service *Service) validatePublishSources(ctx context.Context, config domain.MonitorConfig, rules []domain.MonitorRule, sources []domain.MonitorSource, lock bool) (domain.MonitorConfig, map[int64]sourcedomain.MonitorSourceConnection, error) {
	effective, err := effectiveLocales(config, rules)
	if err != nil {
		return domain.MonitorConfig{}, nil, domain.InvalidMonitorConfiguration()
	}
	facts := make(map[int64]sourcedomain.MonitorSourceConnection, len(sources))
	schedulable := 0
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		var connection sourcedomain.MonitorSourceConnection
		if lock {
			connection, err = service.sources.LockForMonitor(ctx, source.SourceConnectionID)
		} else {
			connection, err = service.sources.FindForMonitor(ctx, source.SourceConnectionID)
		}
		if err != nil {
			return domain.MonitorConfig{}, nil, monitorSourceError(err)
		}
		if !connection.Enabled || connection.Deleted {
			return domain.MonitorConfig{}, nil, sourcedomain.SourceConnectionRequired()
		}
		perSource, err := intersectSourceLocales(effective, connection.Config)
		if err != nil {
			return domain.MonitorConfig{}, nil, domain.InvalidMonitorConfiguration()
		}
		_ = perSource // per-source effective locales are recomputed by querySignature.
		facts[connection.ID] = connection
		schedulable++
	}
	if schedulable == 0 {
		return domain.MonitorConfig{}, nil, sourcedomain.SourceConnectionRequired()
	}
	return effective, facts, nil
}

func effectiveLocales(config domain.MonitorConfig, rules []domain.MonitorRule) (domain.MonitorConfig, error) {
	config, err := domain.NormalizeMonitorConfig(config)
	if err != nil {
		return domain.MonitorConfig{}, err
	}
	languages, regions := append([]string(nil), config.Languages...), append([]string(nil), config.Regions...)
	for _, rule := range rules {
		if !rule.Enabled || rule.ApprovalStatus != domain.RuleApprovalApproved {
			continue
		}
		if rule.RuleType == domain.RuleTypeLanguage {
			languages = applyLocaleRule(languages, rule.Value, rule.Operator)
		}
		if rule.RuleType == domain.RuleTypeRegion {
			regions = applyLocaleRule(regions, rule.Value, rule.Operator)
		}
	}
	if len(languages) == 0 {
		return domain.MonitorConfig{}, fmt.Errorf("language rules exclude every configured language")
	}
	config.Languages, config.Regions = languages, regions
	return config, nil
}
func applyLocaleRule(values []string, target string, operator domain.RuleOperator) []string {
	result := []string{}
	for _, value := range values {
		include := true
		if operator == domain.RuleOperatorEquals {
			include = value == target
		}
		if operator == domain.RuleOperatorNotEquals {
			include = value != target
		}
		if include {
			result = append(result, value)
		}
	}
	return result
}
func intersectSourceLocales(config domain.MonitorConfig, source sourcedomain.SourceConfig) (domain.MonitorConfig, error) {
	var err error
	if len(source.AllowedLanguages) > 0 {
		config.Languages = intersection(config.Languages, source.AllowedLanguages)
		if len(config.Languages) == 0 {
			return domain.MonitorConfig{}, fmt.Errorf("source language intersection is empty")
		}
	}
	if len(source.AllowedRegions) > 0 {
		config.Regions = intersection(config.Regions, source.AllowedRegions)
		if len(config.Regions) == 0 {
			return domain.MonitorConfig{}, fmt.Errorf("source region intersection is empty")
		}
	}
	config.Languages, err = domain.NormalizeLanguages(config.Languages, 1, 8)
	if err != nil {
		return domain.MonitorConfig{}, err
	}
	config.Regions, err = domain.NormalizeRegions(config.Regions, 0, 8)
	return config, err
}
func intersection(left, right []string) []string {
	allowed := map[string]struct{}{}
	for _, value := range right {
		allowed[value] = struct{}{}
	}
	result := []string{}
	for _, value := range left {
		if _, ok := allowed[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

func querySignature(source domain.MonitorSource, connection sourcedomain.MonitorSourceConnection, config domain.MonitorConfig, rules []domain.MonitorRule) (string, error) {
	config, err := intersectSourceLocales(config, connection.Config)
	if err != nil {
		return "", err
	}
	include, exclude := []signatureRule{}, []signatureRule{}
	for _, rule := range rules {
		if !rule.Enabled || rule.ApprovalStatus != domain.RuleApprovalApproved {
			continue
		}
		candidate := signatureRule{Type: rule.RuleType, Operator: rule.Operator, Value: rule.Value, Weight: rule.Weight, Priority: rule.Priority, ID: rule.ID}
		if rule.RuleType == domain.RuleTypeExcludeKeyword || rule.Operator == domain.RuleOperatorNotEquals {
			exclude = append(exclude, candidate)
		} else {
			include = append(include, candidate)
		}
	}
	sort.Slice(include, func(i, j int) bool { return signatureRuleLess(include[i], include[j]) })
	sort.Slice(exclude, func(i, j int) bool { return signatureRuleLess(exclude[i], exclude[j]) })
	override, err := domain.NormalizeQueryOverride(source.QueryOverride)
	if err != nil {
		return "", err
	}
	payload := struct {
		SignatureVersion   int                     `json:"signature_version"`
		SourceConnectionID int64                   `json:"source_connection_id"`
		SourceType         sourcedomain.SourceType `json:"source_type"`
		Endpoint           string                  `json:"normalized_endpoint"`
		Override           string                  `json:"normalized_query_override"`
		Languages          []string                `json:"languages"`
		Regions            []string                `json:"regions"`
		Include            []signatureRule         `json:"include_rules"`
		Exclude            []signatureRule         `json:"exclude_rules"`
		Window             string                  `json:"query_window_kind"`
	}{1, connection.ID, connection.SourceType, connection.Endpoint, override, config.Languages, config.Regions, include, exclude, "scheduled_interval"}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

type signatureRule struct {
	Type     domain.RuleType     `json:"type"`
	Operator domain.RuleOperator `json:"operator"`
	Value    string              `json:"value"`
	Weight   float64             `json:"weight"`
	Priority int16               `json:"priority"`
	ID       int64               `json:"id"`
}

func signatureRuleLess(left, right signatureRule) bool {
	if left.Type != right.Type {
		return left.Type < right.Type
	}
	if left.Operator != right.Operator {
		return left.Operator < right.Operator
	}
	if left.Value != right.Value {
		return left.Value < right.Value
	}
	if left.Weight != right.Weight {
		return left.Weight < right.Weight
	}
	if left.Priority != right.Priority {
		return left.Priority < right.Priority
	}
	return left.ID < right.ID
}

func requireAuthenticated(subject identitydomain.Subject) error {
	if subject.UserID <= 0 || !subject.Role.Valid() {
		return sharederrors.New(sharederrors.CodeUnauthenticated, 401, "")
	}
	return nil
}
func requireEditor(subject identitydomain.Subject) error {
	if err := requireAuthenticated(subject); err != nil {
		return err
	}
	if subject.Role == identitydomain.RoleViewer {
		return sharederrors.New(sharederrors.CodeForbidden, 403, "")
	}
	return nil
}
func requireAdmin(subject identitydomain.Subject) error {
	if err := requireAuthenticated(subject); err != nil {
		return err
	}
	if subject.Role != identitydomain.RoleAdmin {
		return sharederrors.New(sharederrors.CodeForbidden, 403, "")
	}
	return nil
}
func (service *Service) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if service == nil || service.runtime == nil {
		return sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	}
	return service.runtime.WithinTransaction(ctx, fn)
}
func lockConfiguration(ctx context.Context, transaction database.Transaction) error {
	if transaction.SQL == nil {
		return fmt.Errorf("monitor configuration transaction is required")
	}
	_, err := transaction.SQL.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, configurationAdvisoryLock)
	return err
}
func monitorReadError(err error) error {
	if err == nil {
		return nil
	}
	var app *sharederrors.AppError
	if errors.As(err, &app) {
		return app
	}
	return domain.MonitorDraftUnavailable()
}
func monitorWriteError(err error) error {
	if err == nil {
		return nil
	}
	var app *sharederrors.AppError
	if errors.As(err, &app) {
		return app
	}
	if errors.Is(err, sharedrepository.ErrConflict) {
		return domain.MonitorVersionConflict()
	}
	if errors.Is(err, sharedrepository.ErrConstraint) {
		return domain.MonitorNameConflict()
	}
	return err
}
func monitorSourceError(err error) error {
	var app *sharederrors.AppError
	if errors.As(err, &app) {
		return app
	}
	return sourcedomain.SourceConnectionUnavailable()
}
func int64Pointer(value int64) *int64 { return &value }

func (service *Service) auditEntry(ctx context.Context, subject identitydomain.Subject, action operationsdomain.AuditAction, monitor domain.Monitor, config *domain.MonitorConfigVersion, ruleCount, sourceCount int, additional map[string]any) operationsdomain.AuditEntry {
	metadata := map[string]any{"monitor_version": monitor.Version, "status": string(monitor.Status)}
	if config != nil {
		metadata["config_version"], metadata["revision"], metadata["rule_count"], metadata["source_count"] = config.Version, config.Revision, ruleCount, sourceCount
		if config.ConfigHash != "" {
			metadata["config_hash"] = config.ConfigHash
		}
		if config.PublishedAt != nil {
			metadata["published_at"] = config.PublishedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	for key, value := range additional {
		metadata[key] = value
	}
	return operationsdomain.AuditEntry{ActorType: "user", ActorID: subject.UserID, Action: action, ResourceType: "monitor", ResourceID: monitor.ID, RequestID: requestcontext.RequestID(ctx), TraceID: requestcontext.TraceID(ctx), Before: nil, After: metadata, Result: operationsdomain.AuditResultSuccess}
}
