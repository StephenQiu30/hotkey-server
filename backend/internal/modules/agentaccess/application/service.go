package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/agentaccess/domain"
	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	"github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

type Dependencies struct {
	Runtime TransactionRunner
	Tokens  domain.TokenRepository
	Audit   identitydomain.AuditRepository
	Clock   identitydomain.Clock
}

type Service struct {
	runtime TransactionRunner
	tokens  domain.TokenRepository
	audit   identitydomain.AuditRepository
	clock   identitydomain.Clock
}

func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Runtime == nil || dependencies.Tokens == nil || dependencies.Audit == nil || dependencies.Clock == nil {
		return nil, errors.New("agent access dependencies are required")
	}
	return &Service{runtime: dependencies.Runtime, tokens: dependencies.Tokens, audit: dependencies.Audit, clock: dependencies.Clock}, nil
}

type CreateInput struct {
	Subject      identitydomain.Subject
	Name         string
	Scopes       []domain.Scope
	LifetimeDays int
}

type CreatedToken struct {
	Token domain.Token
	Raw   string
}

type RevokeInput struct {
	Subject         identitydomain.Subject
	TokenID         int64
	ExpectedVersion int64
}

func (service *Service) Create(ctx context.Context, input CreateInput) (CreatedToken, error) {
	if service == nil || service.runtime == nil || service.tokens == nil || service.audit == nil || service.clock == nil {
		return CreatedToken{}, unavailable(nil)
	}
	if !input.Subject.Authenticated() {
		return CreatedToken{}, unauthenticated()
	}
	scopes, err := domain.NormalizeScopes(input.Scopes)
	if err != nil {
		return CreatedToken{}, invalidRequest()
	}
	if !domain.ScopesAllowedForRole(input.Subject.Role, scopes) {
		return CreatedToken{}, forbidden()
	}
	if input.LifetimeDays < 1 || input.LifetimeDays > 90 {
		return CreatedToken{}, invalidRequest()
	}
	now := service.clock.Now().UTC()
	raw, prefix, digest, err := newCredential()
	if err != nil {
		return CreatedToken{}, unavailable(err)
	}
	token := domain.Token{
		UserID: input.Subject.UserID, Name: input.Name, TokenPrefix: prefix,
		TokenHash: digest, Scopes: scopes, ExpiresAt: now.Add(time.Duration(input.LifetimeDays) * 24 * time.Hour),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := token.Validate(now); err != nil {
		return CreatedToken{}, invalidRequest()
	}
	err = service.runtime.RunInTransaction(ctx, func(transactionCtx context.Context) error {
		count, err := service.tokens.CountActive(transactionCtx, input.Subject.UserID, now)
		if err != nil {
			return err
		}
		if count >= domain.MaximumActiveTokens {
			return sharedrepository.ErrConflict
		}
		if err := service.tokens.Create(transactionCtx, &token); err != nil {
			return err
		}
		return service.audit.Create(transactionCtx, auditEntry(transactionCtx, input.Subject.UserID, "agent_token.created", token.ID, "success", map[string]any{"status": "active"}))
	})
	if err != nil {
		return CreatedToken{}, serviceError(err)
	}
	return CreatedToken{Token: token, Raw: raw}, nil
}

func (service *Service) List(ctx context.Context, subject identitydomain.Subject) ([]domain.Token, error) {
	if service == nil || service.tokens == nil || !subject.Authenticated() {
		return nil, unauthenticated()
	}
	tokens, err := service.tokens.ListByUser(ctx, subject.UserID)
	if err != nil {
		return nil, serviceError(err)
	}
	return tokens, nil
}

func (service *Service) Revoke(ctx context.Context, input RevokeInput) (*domain.Token, error) {
	if service == nil || service.runtime == nil || service.tokens == nil || service.audit == nil || service.clock == nil {
		return nil, unavailable(nil)
	}
	if !input.Subject.Authenticated() {
		return nil, unauthenticated()
	}
	if input.TokenID <= 0 || input.ExpectedVersion <= 0 {
		return nil, invalidRequest()
	}
	now := service.clock.Now().UTC()
	var revoked *domain.Token
	err := service.runtime.RunInTransaction(ctx, func(transactionCtx context.Context) error {
		var err error
		revoked, err = service.tokens.Revoke(transactionCtx, input.Subject.UserID, input.TokenID, input.ExpectedVersion, now)
		if err != nil {
			return err
		}
		return service.audit.Create(transactionCtx, auditEntry(transactionCtx, input.Subject.UserID, "agent_token.revoked", tokenID(revoked), "success", map[string]any{"status": "revoked"}))
	})
	if err != nil {
		return nil, serviceError(err)
	}
	return revoked, nil
}

func (service *Service) Authenticate(ctx context.Context, raw string) (domain.Principal, error) {
	if service == nil || service.tokens == nil || service.clock == nil || !strings.HasPrefix(raw, "hk_agent_") {
		return domain.Principal{}, unauthenticated()
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	principal, err := service.tokens.Authenticate(ctx, hex.EncodeToString(digest[:]), service.clock.Now().UTC())
	if err != nil {
		if errors.Is(err, sharedrepository.ErrNotFound) {
			return domain.Principal{}, unauthenticated()
		}
		return domain.Principal{}, unavailable(err)
	}
	if principal.TokenID <= 0 || principal.UserID <= 0 || !principal.Role.Valid() {
		return domain.Principal{}, unauthenticated()
	}
	scopes, err := domain.NormalizeScopes(principal.Scopes)
	if err != nil {
		return domain.Principal{}, unauthenticated()
	}
	principal.Scopes = scopes
	return principal, nil
}

func newCredential() (raw, prefix, digest string, err error) {
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return "", "", "", err
	}
	raw = "hk_agent_" + base64.RawURLEncoding.EncodeToString(secret)
	prefix = raw[:19]
	hash := sha256.Sum256([]byte(raw))
	return raw, prefix, hex.EncodeToString(hash[:]), nil
}

func tokenID(token *domain.Token) int64 {
	if token == nil {
		return 0
	}
	return token.ID
}

func auditEntry(ctx context.Context, actorID int64, action string, resourceID int64, result string, after map[string]any) identitydomain.AuditEntry {
	return identitydomain.AuditEntry{
		ActorType: "user", ActorID: actorID, Action: action, ResourceType: "agent_token", ResourceID: resourceID,
		RequestID: requestcontext.RequestID(ctx), TraceID: requestcontext.TraceID(ctx), Result: result, AfterData: after,
	}
}

func serviceError(err error) error {
	var appError *sharederrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	switch {
	case errors.Is(err, sharedrepository.ErrInvalidInput), errors.Is(err, sharedrepository.ErrConstraint):
		return invalidRequest()
	case errors.Is(err, sharedrepository.ErrNotFound):
		return sharederrors.New(sharederrors.CodeNotFound, http.StatusNotFound, "agent token not found")
	case errors.Is(err, sharedrepository.ErrConflict):
		return sharederrors.New(sharederrors.CodeConflict, http.StatusConflict, "agent token conflict")
	case errors.Is(err, sharedrepository.ErrUnavailable):
		return unavailable(err)
	default:
		return unavailable(fmt.Errorf("agent token operation: %w", err))
	}
}

func invalidRequest() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeInvalidRequest, http.StatusBadRequest, "invalid agent token request")
}

func unauthenticated() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeUnauthenticated, http.StatusUnauthorized, "")
}

func forbidden() *sharederrors.AppError {
	return sharederrors.New(sharederrors.CodeForbidden, http.StatusForbidden, "")
}

func unavailable(cause error) *sharederrors.AppError {
	return sharederrors.Wrap(sharederrors.CodeUnavailable, http.StatusServiceUnavailable, "", cause)
}
