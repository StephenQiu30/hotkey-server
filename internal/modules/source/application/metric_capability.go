package application

import (
	"context"
	"errors"
	"regexp"
	"strings"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/internal/shared/requestcontext"
)

var metricCapabilityReasonPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type MetricCapabilityDependencies struct {
	Runtime  *database.Runtime
	Profiles domain.MetricCapabilityProfileRepository
	Audit    operationsapplication.AuditWriter
}

type MetricCapabilityService struct {
	runtime  *database.Runtime
	profiles domain.MetricCapabilityProfileRepository
	audit    operationsapplication.AuditWriter
}

var _ domain.MetricCapabilityReader = (*MetricCapabilityService)(nil)

func NewMetricCapabilityService(dependencies MetricCapabilityDependencies) (*MetricCapabilityService, error) {
	if dependencies.Runtime == nil || dependencies.Profiles == nil || dependencies.Audit == nil {
		return nil, errors.New("metric capability dependencies are required")
	}
	return &MetricCapabilityService{runtime: dependencies.Runtime, profiles: dependencies.Profiles, audit: dependencies.Audit}, nil
}

type CreateMetricCapabilityInput struct {
	Subject identitydomain.Subject
	Profile domain.MetricCapabilityProfile
}

type MetricCapabilityLifecycleInput struct {
	Subject         identitydomain.Subject
	ID              int64
	ExpectedVersion int64
	ReasonCode      string
}

func (service *MetricCapabilityService) CreateDraft(ctx context.Context, input CreateMetricCapabilityInput) (*domain.MetricCapabilityProfile, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, err
	}
	if err := input.Profile.ValidateDraft(); err != nil {
		return nil, sharederrors.New(sharederrors.CodeInvalidRequest, 400, "")
	}
	profile := input.Profile
	profile.Status, profile.PublishedAt, profile.ArchivedAt = domain.MetricCapabilityDraft, nil, nil
	err := service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := service.profiles.CreateDraft(ctx, &profile); err != nil {
			return metricCapabilityWriteError(err)
		}
		return service.audit.Write(ctx, metricCapabilityAuditEntry(ctx, input.Subject, operationsdomain.ActionMetricCapabilityDrafted, profile, "draft_created"))
	})
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (service *MetricCapabilityService) Publish(ctx context.Context, input MetricCapabilityLifecycleInput) (*domain.MetricCapabilityProfile, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, err
	}
	reason, err := normalizeMetricCapabilityReason(input)
	if err != nil {
		return nil, err
	}
	var published domain.MetricCapabilityProfile
	err = service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		profile, err := service.profiles.LockByID(ctx, input.ID)
		if err != nil {
			return metricCapabilityReadError(err)
		}
		if profile.Version != input.ExpectedVersion || profile.Status != domain.MetricCapabilityDraft {
			return sharederrors.New(sharederrors.CodeConflict, 409, "")
		}
		current, err := service.profiles.LockPublished(ctx, profile.SourceType)
		if err != nil && !errors.Is(err, sharedrepository.ErrNotFound) {
			return metricCapabilityReadError(err)
		}
		if current != nil {
			if err := service.profiles.Archive(ctx, current); err != nil {
				return metricCapabilityWriteError(err)
			}
			if err := service.audit.Write(ctx, metricCapabilityAuditEntry(ctx, input.Subject, operationsdomain.ActionMetricCapabilityArchived, *current, "superseded")); err != nil {
				return err
			}
		}
		if err := service.profiles.Publish(ctx, profile); err != nil {
			return metricCapabilityWriteError(err)
		}
		if err := service.audit.Write(ctx, metricCapabilityAuditEntry(ctx, input.Subject, operationsdomain.ActionMetricCapabilityPublished, *profile, reason)); err != nil {
			return err
		}
		published = *profile
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &published, nil
}

func (service *MetricCapabilityService) Archive(ctx context.Context, input MetricCapabilityLifecycleInput) (*domain.MetricCapabilityProfile, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, err
	}
	reason, err := normalizeMetricCapabilityReason(input)
	if err != nil {
		return nil, err
	}
	var archived domain.MetricCapabilityProfile
	err = service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		profile, err := service.profiles.LockByID(ctx, input.ID)
		if err != nil {
			return metricCapabilityReadError(err)
		}
		if profile.Version != input.ExpectedVersion {
			return sharederrors.New(sharederrors.CodeConflict, 409, "")
		}
		if profile.Status == domain.MetricCapabilityArchived {
			archived = *profile
			return nil
		}
		if err := service.profiles.Archive(ctx, profile); err != nil {
			return metricCapabilityWriteError(err)
		}
		if err := service.audit.Write(ctx, metricCapabilityAuditEntry(ctx, input.Subject, operationsdomain.ActionMetricCapabilityArchived, *profile, reason)); err != nil {
			return err
		}
		archived = *profile
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &archived, nil
}

func (service *MetricCapabilityService) FindPublishedMetricCapability(ctx context.Context, sourceType domain.SourceType) (domain.MetricCapabilityProfile, error) {
	if service == nil || service.profiles == nil {
		return domain.MetricCapabilityProfile{}, sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	}
	profile, err := service.profiles.FindPublished(ctx, sourceType)
	if err != nil {
		return domain.MetricCapabilityProfile{}, metricCapabilityReadError(err)
	}
	return *profile, nil
}

func (service *MetricCapabilityService) withTransaction(ctx context.Context, fn func(context.Context, database.Transaction) error) error {
	if service == nil || service.runtime == nil {
		return sharederrors.New(sharederrors.CodeUnavailable, 503, "")
	}
	return service.runtime.WithinTransaction(ctx, fn)
}

func normalizeMetricCapabilityReason(input MetricCapabilityLifecycleInput) (string, error) {
	if input.ID <= 0 || input.ExpectedVersion <= 0 || !metricCapabilityReasonPattern.MatchString(strings.TrimSpace(input.ReasonCode)) {
		return "", sharederrors.New(sharederrors.CodeInvalidRequest, 400, "")
	}
	return strings.TrimSpace(input.ReasonCode), nil
}

func metricCapabilityReadError(err error) error {
	if errors.Is(err, sharedrepository.ErrNotFound) {
		return sharederrors.New(sharederrors.CodeNotFound, 404, "")
	}
	if errors.Is(err, sharedrepository.ErrConflict) || errors.Is(err, sharedrepository.ErrConstraint) {
		return sharederrors.New(sharederrors.CodeConflict, 409, "")
	}
	if errors.Is(err, sharedrepository.ErrInvalidInput) {
		return sharederrors.New(sharederrors.CodeInvalidRequest, 400, "")
	}
	return sharederrors.New(sharederrors.CodeUnavailable, 503, "")
}

func metricCapabilityWriteError(err error) error {
	if errors.Is(err, sharedrepository.ErrConflict) || errors.Is(err, sharedrepository.ErrConstraint) {
		return sharederrors.New(sharederrors.CodeConflict, 409, "")
	}
	if errors.Is(err, sharedrepository.ErrInvalidInput) {
		return sharederrors.New(sharederrors.CodeInvalidRequest, 400, "")
	}
	return metricCapabilityReadError(err)
}

func metricCapabilityAuditEntry(ctx context.Context, subject identitydomain.Subject, action operationsdomain.AuditAction, profile domain.MetricCapabilityProfile, reasonCode string) operationsdomain.AuditEntry {
	metadata := map[string]any{
		"capability_source_type": string(profile.SourceType), "capability_profile_version": profile.ProfileVersion,
		"capability_status": string(profile.Status), "capability_profile_record_version": profile.Version, "reason_code": reasonCode,
	}
	return operationsdomain.AuditEntry{ActorType: "user", ActorID: subject.UserID, Action: action, ResourceType: "metric_capability_profile", ResourceID: profile.ID,
		RequestID: requestcontext.RequestID(ctx), TraceID: requestcontext.TraceID(ctx), After: metadata, Result: operationsdomain.AuditResultSuccess}
}
