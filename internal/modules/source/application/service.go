// Package application coordinates SourceConnection administration. It owns
// authorization, optimistic version checks, the configuration advisory lock,
// and the atomic business/audit boundary; it never issues Monitor SQL.
package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	identitydomain "github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	operationsapplication "github.com/StephenQiu30/hotkey-server/internal/modules/operations/application"
	operationsdomain "github.com/StephenQiu30/hotkey-server/internal/modules/operations/domain"
	"github.com/StephenQiu30/hotkey-server/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	sharederrors "github.com/StephenQiu30/hotkey-server/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/internal/shared/requestcontext"
)

const configurationAdvisoryLock = "hotkey.monitor_source_configuration"

type Dependencies struct {
	Runtime             *database.Runtime
	Sources             domain.SourceConnectionRepository
	MonitorUsage        domain.MonitorUsageReader
	PublishedReferences domain.MonitorPublishedReferenceReader
	Audit               operationsapplication.AuditWriter
}

type Service struct {
	runtime             *database.Runtime
	sources             domain.SourceConnectionRepository
	monitorUsage        domain.MonitorUsageReader
	publishedReferences domain.MonitorPublishedReferenceReader
	audit               operationsapplication.AuditWriter
}

var (
	_ domain.MonitorSourceReader = (*Service)(nil)
	_ domain.ContentSourceReader = (*Service)(nil)
)

func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Runtime == nil || dependencies.Sources == nil || dependencies.MonitorUsage == nil || dependencies.PublishedReferences == nil || dependencies.Audit == nil {
		return nil, errors.New("source application dependencies are required")
	}
	return &Service{
		runtime: dependencies.Runtime, sources: dependencies.Sources,
		monitorUsage: dependencies.MonitorUsage, publishedReferences: dependencies.PublishedReferences,
		audit: dependencies.Audit,
	}, nil
}

type CreateInput struct {
	Subject    identitydomain.Subject
	Connection domain.SourceConnection
}

// UpdateInput uses pointers so a transport can distinguish a PATCH omission
// from an intentional zero value. Its Config is already a typed allowlisted
// value, never request JSON.
type UpdateInput struct {
	Subject         identitydomain.Subject
	ID              int64
	ExpectedVersion int64
	Name            *string
	SourceType      *domain.SourceType
	Endpoint        *string
	AuthType        *domain.AuthType
	CredentialRef   *string
	Config          *domain.SourceConfig
	TermsPolicyURL  *string
}

type LifecycleInput struct {
	Subject         identitydomain.Subject
	ID              int64
	ExpectedVersion int64
}

type ListInput struct {
	Subject identitydomain.Subject
	Query   domain.SourceConnectionListQuery
}

func (service *Service) Create(ctx context.Context, input CreateInput) (*domain.ManagementSourceConnection, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, err
	}
	connection, err := normalizeCreate(input.Connection)
	if err != nil {
		return nil, err
	}
	var created domain.SourceConnection
	err = service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockConfiguration(ctx, transaction); err != nil {
			return err
		}
		if err := service.sources.Create(ctx, &connection); err != nil {
			return sourceWriteError(err)
		}
		if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, operationsdomain.ActionSourceCreated, connection.ID, nil, sourceMetadata(connection))); err != nil {
			return err
		}
		created = connection
		return nil
	})
	if err != nil {
		return nil, err
	}
	management := managementProjection(created)
	return &management, nil
}

func (service *Service) Update(ctx context.Context, input UpdateInput) (*domain.ManagementSourceConnection, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, err
	}
	if input.ID <= 0 || input.ExpectedVersion <= 0 {
		return nil, domain.SourceConnectionUnavailable()
	}
	var changed domain.SourceConnection
	err := service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockConfiguration(ctx, transaction); err != nil {
			return err
		}
		current, err := service.sources.LockByID(ctx, input.ID)
		if err != nil {
			return sourceReadError(err)
		}
		if current.Version != input.ExpectedVersion || current.Deleted {
			return domain.SourceConnectionUnavailable()
		}
		next, semanticChanged, credentialChanged, err := mergeUpdate(*current, input)
		if err != nil {
			return err
		}
		if semanticChanged {
			referenced, err := service.publishedReferences.HasPublishedReference(ctx, current.ID)
			if err != nil {
				return sourceReadError(err)
			}
			if referenced && !isBodyStorageAuthorizationUpgrade(*current, next) {
				return domain.SourceConnectionUnavailable()
			}
		}
		if credentialChanged || semanticChanged {
			next.HealthStatus = domain.HealthStatusUnknown
		}
		before := sourceMetadata(*current)
		if err := service.sources.Update(ctx, &next); err != nil {
			return sourceWriteError(err)
		}
		if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, operationsdomain.ActionSourceUpdated, next.ID, before, sourceMetadata(next))); err != nil {
			return err
		}
		changed = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	management := managementProjection(changed)
	return &management, nil
}

func (service *Service) Enable(ctx context.Context, input LifecycleInput) (*domain.ManagementSourceConnection, error) {
	return service.changeEnabled(ctx, input, true)
}

func (service *Service) Disable(ctx context.Context, input LifecycleInput) (*domain.ManagementSourceConnection, error) {
	return service.changeEnabled(ctx, input, false)
}

func (service *Service) changeEnabled(ctx context.Context, input LifecycleInput, enabled bool) (*domain.ManagementSourceConnection, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, err
	}
	if input.ID <= 0 || input.ExpectedVersion <= 0 {
		return nil, domain.SourceConnectionUnavailable()
	}
	var changed domain.SourceConnection
	err := service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockConfiguration(ctx, transaction); err != nil {
			return err
		}
		current, err := service.sources.LockByID(ctx, input.ID)
		if err != nil {
			return sourceReadError(err)
		}
		if current.Version != input.ExpectedVersion || current.Deleted {
			return domain.SourceConnectionUnavailable()
		}
		if current.Enabled == enabled {
			changed = *current
			return nil
		}
		if !enabled {
			if err := service.ensureCanRemoveSchedulableSource(ctx, current.ID); err != nil {
				return err
			}
		}
		next := *current
		next.Enabled = enabled
		before := sourceMetadata(*current)
		if err := service.sources.Update(ctx, &next); err != nil {
			return sourceWriteError(err)
		}
		action := operationsdomain.ActionSourceDisabled
		if enabled {
			action = operationsdomain.ActionSourceEnabled
		}
		if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, action, next.ID, before, sourceMetadata(next))); err != nil {
			return err
		}
		changed = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	management := managementProjection(changed)
	return &management, nil
}

func (service *Service) Archive(ctx context.Context, input LifecycleInput) (*domain.ManagementSourceConnection, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, err
	}
	if input.ID <= 0 || input.ExpectedVersion <= 0 {
		return nil, domain.SourceConnectionUnavailable()
	}
	var changed domain.SourceConnection
	err := service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockConfiguration(ctx, transaction); err != nil {
			return err
		}
		current, err := service.sources.LockByID(ctx, input.ID)
		if err != nil {
			return sourceReadError(err)
		}
		if current.Version != input.ExpectedVersion {
			return domain.SourceConnectionUnavailable()
		}
		if current.Deleted {
			changed = *current
			return nil
		}
		if err := service.ensureCanRemoveSchedulableSource(ctx, current.ID); err != nil {
			return err
		}
		next := *current
		next.Enabled = false
		next.Deleted = true
		before := sourceMetadata(*current)
		if err := service.sources.Update(ctx, &next); err != nil {
			return sourceWriteError(err)
		}
		if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, operationsdomain.ActionSourceArchived, next.ID, before, sourceMetadata(next))); err != nil {
			return err
		}
		changed = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	management := managementProjection(changed)
	return &management, nil
}

func (service *Service) Restore(ctx context.Context, input LifecycleInput) (*domain.ManagementSourceConnection, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return nil, err
	}
	if input.ID <= 0 || input.ExpectedVersion <= 0 {
		return nil, domain.SourceConnectionUnavailable()
	}
	var changed domain.SourceConnection
	err := service.withTransaction(ctx, func(ctx context.Context, transaction database.Transaction) error {
		if err := lockConfiguration(ctx, transaction); err != nil {
			return err
		}
		current, err := service.sources.LockByID(ctx, input.ID)
		if err != nil {
			return sourceReadError(err)
		}
		if current.Version != input.ExpectedVersion {
			return domain.SourceConnectionUnavailable()
		}
		if !current.Deleted {
			changed = *current
			return nil
		}
		next := *current
		next.Deleted = false
		next.Enabled = false
		next.HealthStatus = domain.HealthStatusUnknown
		before := sourceMetadata(*current)
		if err := service.sources.Update(ctx, &next); err != nil {
			return sourceWriteError(err)
		}
		if err := service.audit.Write(ctx, service.auditEntry(ctx, input.Subject, operationsdomain.ActionSourceRestored, next.ID, before, sourceMetadata(next))); err != nil {
			return err
		}
		changed = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	management := managementProjection(changed)
	return &management, nil
}

// GetPublic is safe for every authenticated role. It deliberately has no
// management fields, even if the caller happens to be an administrator.
func (service *Service) GetPublic(ctx context.Context, subject identitydomain.Subject, id int64) (domain.PublicSourceConnection, error) {
	if err := requireAuthenticated(subject); err != nil {
		return domain.PublicSourceConnection{}, err
	}
	connection, err := service.sources.FindByID(ctx, id)
	if err != nil {
		return domain.PublicSourceConnection{}, sourceReadError(err)
	}
	return publicProjection(*connection), nil
}

func (service *Service) GetManagement(ctx context.Context, subject identitydomain.Subject, id int64) (domain.ManagementSourceConnection, error) {
	if err := requireAdmin(subject); err != nil {
		return domain.ManagementSourceConnection{}, err
	}
	connection, err := service.sources.FindByID(ctx, id)
	if err != nil {
		return domain.ManagementSourceConnection{}, sourceReadError(err)
	}
	return managementProjection(*connection), nil
}

// ListPublic exposes the same deliberately safe view as GetPublic. It is
// authenticated rather than owner-scoped because SourceConnection is a shared
// team resource by design.
func (service *Service) ListPublic(ctx context.Context, input ListInput) (domain.PublicSourceConnectionPage, error) {
	if err := requireAuthenticated(input.Subject); err != nil {
		return domain.PublicSourceConnectionPage{}, err
	}
	connections, nextCursor, err := service.sources.List(ctx, input.Query)
	if err != nil {
		return domain.PublicSourceConnectionPage{}, sourceReadError(err)
	}
	items := make([]domain.PublicSourceConnection, 0, len(connections))
	for _, connection := range connections {
		items = append(items, publicProjection(connection))
	}
	return domain.PublicSourceConnectionPage{Items: items, NextCursor: nextCursor}, nil
}

// ListManagement retains endpoint and validated non-secret configuration only
// for administrators; the credential reference is absent from every item.
func (service *Service) ListManagement(ctx context.Context, input ListInput) (domain.ManagementSourceConnectionPage, error) {
	if err := requireAdmin(input.Subject); err != nil {
		return domain.ManagementSourceConnectionPage{}, err
	}
	connections, nextCursor, err := service.sources.List(ctx, input.Query)
	if err != nil {
		return domain.ManagementSourceConnectionPage{}, sourceReadError(err)
	}
	items := make([]domain.ManagementSourceConnection, 0, len(connections))
	for _, connection := range connections {
		items = append(items, managementProjection(connection))
	}
	return domain.ManagementSourceConnectionPage{Items: items, NextCursor: nextCursor}, nil
}

func (service *Service) FindForMonitor(ctx context.Context, id int64) (domain.MonitorSourceConnection, error) {
	connection, err := service.sources.FindByID(ctx, id)
	if err != nil {
		return domain.MonitorSourceConnection{}, sourceReadError(err)
	}
	return monitorProjection(*connection), nil
}

func (service *Service) LockForMonitor(ctx context.Context, id int64) (domain.MonitorSourceConnection, error) {
	connection, err := service.sources.LockByID(ctx, id)
	if err != nil {
		return domain.MonitorSourceConnection{}, sourceReadError(err)
	}
	return monitorProjection(*connection), nil
}

// FindForContent is the credential-free Source application boundary for
// ingestion's safe Content read model. A deleted source is returned as a
// tombstone flag so the consuming application can suppress stale Content
// rather than infer Source lifecycle state from a database table.
func (service *Service) FindForContent(ctx context.Context, id int64) (domain.ContentSourceReference, error) {
	connection, err := service.sources.FindByID(ctx, id)
	if err != nil {
		return domain.ContentSourceReference{}, sourceReadError(err)
	}
	return domain.ContentSourceReference{Name: connection.Name, SourceType: connection.SourceType, Deleted: connection.Deleted}, nil
}

func (service *Service) ensureCanRemoveSchedulableSource(ctx context.Context, sourceID int64) error {
	usage, err := service.monitorUsage.UsageForSource(ctx, sourceID)
	if err != nil {
		return sourceReadError(err)
	}
	// Monitor owns the association groups, but it must not decide whether a
	// different SourceConnection is currently executable. Resolve that narrow
	// Source-owned predicate under this same transaction and advisory lock.
	for _, group := range usage.ActiveMonitorGroups {
		hasAlternative := false
		for _, association := range group.Sources {
			if !association.Enabled || association.SourceConnectionID == sourceID {
				continue
			}
			connection, err := service.sources.LockByID(ctx, association.SourceConnectionID)
			if err != nil {
				return sourceReadError(err)
			}
			if connection.Enabled && !connection.Deleted {
				hasAlternative = true
				break
			}
		}
		if !hasAlternative {
			return domain.SourceConnectionRequired()
		}
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
		return fmt.Errorf("source configuration transaction is required")
	}
	_, err := transaction.SQL.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, configurationAdvisoryLock)
	return err
}

func normalizeCreate(connection domain.SourceConnection) (domain.SourceConnection, error) {
	if connection.SourceType != domain.SourceTypeRSS && connection.SourceType != domain.SourceTypeHackerNews {
		return domain.SourceConnection{}, domain.UnsupportedSourceType()
	}
	// A new connection cannot be created already archived. `enabled` remains
	// the requested valid state, while lifecycle archive/restore is the sole
	// route that may change `Deleted` after creation.
	connection.Deleted = false
	connection.HealthStatus = domain.HealthStatusUnknown
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil {
		return domain.SourceConnection{}, domain.InvalidSourceConfiguration()
	}
	return normalized, nil
}

func mergeUpdate(current domain.SourceConnection, input UpdateInput) (domain.SourceConnection, bool, bool, error) {
	next := current
	if input.Name != nil {
		next.Name = *input.Name
	}
	if input.SourceType != nil {
		next.SourceType = *input.SourceType
	}
	if input.Endpoint != nil {
		next.Endpoint = *input.Endpoint
	}
	if input.AuthType != nil {
		next.AuthType = *input.AuthType
	}
	if input.CredentialRef != nil {
		next.CredentialRef = *input.CredentialRef
	}
	if input.Config != nil {
		next.Config = *input.Config
	}
	if input.TermsPolicyURL != nil {
		next.TermsPolicyURL = *input.TermsPolicyURL
	}
	if next.SourceType != domain.SourceTypeRSS && next.SourceType != domain.SourceTypeHackerNews {
		return domain.SourceConnection{}, false, false, domain.UnsupportedSourceType()
	}
	normalized, err := domain.NormalizeSourceConnection(next)
	if err != nil {
		return domain.SourceConnection{}, false, false, domain.InvalidSourceConfiguration()
	}
	semanticChanged := normalized.SourceType != current.SourceType || normalized.Endpoint != current.Endpoint || !sourceConfigsEqual(normalized.Config, current.Config)
	credentialChanged := normalized.AuthType != current.AuthType || normalized.CredentialRef != current.CredentialRef
	return normalized, semanticChanged, credentialChanged, nil
}

func sourceConfigsEqual(left, right domain.SourceConfig) bool {
	// The domain normalizer sorts/deduplicates its arrays, making structural
	// equality a stable semantic boundary rather than a JSON-map comparison.
	return reflect.DeepEqual(left, right)
}

// isBodyStorageAuthorizationUpgrade is the only source semantic change that
// may be applied to a published source. It cannot alter endpoint, source
// type, filtering, retention, rate limits, or any other collection input; it
// only turns on explicit persistence of content already supplied by the Feed.
// This lets an administrator repair a metadata-only source without silently
// changing which items a published monitor collects.
func isBodyStorageAuthorizationUpgrade(current, next domain.SourceConnection) bool {
	if current.SourceType != next.SourceType || current.Endpoint != next.Endpoint || current.Config.AllowBodyStorage || !next.Config.AllowBodyStorage {
		return false
	}
	left, right := current.Config, next.Config
	left.AllowBodyStorage = false
	right.AllowBodyStorage = false
	return sourceConfigsEqual(left, right)
}

func requireAuthenticated(subject identitydomain.Subject) error {
	if subject.UserID <= 0 || !subject.Role.Valid() {
		return sharederrors.New(sharederrors.CodeUnauthenticated, 401, "")
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

func sourceReadError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	if errors.Is(err, sharedrepository.ErrInvalidInput) {
		return sharederrors.New(sharederrors.CodeInvalidRequest, 400, "")
	}
	return domain.SourceConnectionUnavailable()
}

func sourceWriteError(err error) error {
	if err == nil {
		return nil
	}
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	if errors.Is(err, sharedrepository.ErrInvalidInput) {
		return domain.InvalidSourceConfiguration()
	}
	if errors.Is(err, sharedrepository.ErrConflict) || errors.Is(err, sharedrepository.ErrNotFound) || errors.Is(err, sharedrepository.ErrConstraint) {
		return domain.SourceConnectionUnavailable()
	}
	return err
}

func publicProjection(connection domain.SourceConnection) domain.PublicSourceConnection {
	return domain.PublicSourceConnection{ID: connection.ID, Version: connection.Version, Name: connection.Name, SourceType: connection.SourceType,
		Enabled: connection.Enabled, HealthStatus: connection.HealthStatus, TermsPolicyURL: connection.TermsPolicyURL,
		CredentialConfigured: connection.CredentialRef != "", Deleted: connection.Deleted}
}

func managementProjection(connection domain.SourceConnection) domain.ManagementSourceConnection {
	return domain.ManagementSourceConnection{PublicSourceConnection: publicProjection(connection), Endpoint: connection.Endpoint, Config: connection.Config}
}

func monitorProjection(connection domain.SourceConnection) domain.MonitorSourceConnection {
	return domain.MonitorSourceConnection{ID: connection.ID, Version: connection.Version, Name: connection.Name, SourceType: connection.SourceType,
		Endpoint: connection.Endpoint, Config: connection.Config, Enabled: connection.Enabled, Deleted: connection.Deleted}
}

func sourceMetadata(connection domain.SourceConnection) map[string]any {
	return map[string]any{"source_version": connection.Version, "enabled": connection.Enabled, "deleted": connection.Deleted, "credential_configured": connection.CredentialRef != ""}
}

func (service *Service) auditEntry(ctx context.Context, subject identitydomain.Subject, action operationsdomain.AuditAction, sourceID int64, before, after map[string]any) operationsdomain.AuditEntry {
	return operationsdomain.AuditEntry{ActorType: "user", ActorID: subject.UserID, Action: action, ResourceType: "source_connection", ResourceID: sourceID,
		RequestID: requestcontext.RequestID(ctx), TraceID: requestcontext.TraceID(ctx), Before: before, After: after, Result: operationsdomain.AuditResultSuccess}
}
