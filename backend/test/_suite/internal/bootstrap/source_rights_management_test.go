package bootstrap

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	identitydomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/identity/domain"
	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sharederrors "github.com/StephenQiu30/hotkey-server/backend/internal/shared/errors"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
	sharedrequestcontext "github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

func TestRightsActorAuthorizerUsesDurableActiveAdministratorState(t *testing.T) {
	now := time.Now().UTC()
	deletedAt := now
	reader := &rightsActorReaderFake{users: map[int64]*identitydomain.User{
		1: {ID: 1, Role: identitydomain.RoleAdmin, Status: identitydomain.UserStatusActive},
		2: {ID: 2, Role: identitydomain.RoleEditor, Status: identitydomain.UserStatusActive},
		3: {ID: 3, Role: identitydomain.RoleAdmin, Status: identitydomain.UserStatusDisabled},
		4: {ID: 4, Role: identitydomain.RoleAdmin, Status: identitydomain.UserStatusActive, DeletedAt: &deletedAt},
		5: {ID: 5, Role: identitydomain.RoleAdmin, Status: identitydomain.UserStatusActive},
	}}
	authorizer := &rightsActorAuthorizer{users: reader}
	sourceID, approverID := int64(9), int64(5)
	if err := authorizer.AuthorizeRightsMutation(context.Background(), sourceapplication.RightsActorAuthorizationDTO{
		ActorID: 1, Operation: sourceapplication.RightsManagementCreatePolicy,
		SourceConnectionID: &sourceID, ApprovedByUserID: &approverID,
	}); err != nil {
		t.Fatalf("active administrator authorization error = %v", err)
	}
	if !reflect.DeepEqual(reader.readIDs, []int64{1, 5}) {
		t.Fatalf("durable identities read = %v", reader.readIDs)
	}

	for _, actorID := range []int64{2, 3, 4, 99} {
		err := authorizer.AuthorizeRightsMutation(context.Background(), sourceapplication.RightsActorAuthorizationDTO{
			ActorID: actorID, Operation: sourceapplication.RightsManagementRecordDecisions, SourceConnectionID: &sourceID,
		})
		var appError *sharederrors.AppError
		if !errors.As(err, &appError) || appError.Code != sharederrors.CodeForbidden {
			t.Fatalf("actor %d authorization error = %v, want forbidden", actorID, err)
		}
	}
	badApprover := int64(2)
	err := authorizer.AuthorizeRightsMutation(context.Background(), sourceapplication.RightsActorAuthorizationDTO{
		ActorID: 1, Operation: sourceapplication.RightsManagementCreatePolicy,
		SourceConnectionID: &sourceID, ApprovedByUserID: &badApprover,
	})
	var appError *sharederrors.AppError
	if !errors.As(err, &appError) || appError.Code != sharederrors.CodeForbidden {
		t.Fatalf("non-administrator approver error = %v, want forbidden", err)
	}
}

func TestRightsActorAuthorizerRejectsMalformedApplicationDTO(t *testing.T) {
	authorizer := &rightsActorAuthorizer{users: &rightsActorReaderFake{users: map[int64]*identitydomain.User{
		1: {ID: 1, Role: identitydomain.RoleAdmin, Status: identitydomain.UserStatusActive},
	}}}
	for _, authorization := range []sourceapplication.RightsActorAuthorizationDTO{
		{ActorID: 0, Operation: sourceapplication.RightsManagementCreatePolicy},
		{ActorID: 1, Operation: "invented"},
		{ActorID: 1, Operation: sourceapplication.RightsManagementRecordDecisions},
	} {
		if err := authorizer.AuthorizeRightsMutation(context.Background(), authorization); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("AuthorizeRightsMutation(%#v) error = %v, want invalid input", authorization, err)
		}
	}
}

func TestRightsReadAuthorizerUsesDurableRoleAndLifecycleState(t *testing.T) {
	deletedAt := time.Now().UTC()
	reader := &rightsActorReaderFake{users: map[int64]*identitydomain.User{
		1: {ID: 1, Role: identitydomain.RoleAdmin, Status: identitydomain.UserStatusActive},
		2: {ID: 2, Role: identitydomain.RoleEditor, Status: identitydomain.UserStatusActive},
		3: {ID: 3, Role: identitydomain.RoleViewer, Status: identitydomain.UserStatusActive},
		4: {ID: 4, Role: identitydomain.RoleAdmin, Status: identitydomain.UserStatusDisabled},
		5: {ID: 5, Role: identitydomain.RoleAdmin, Status: identitydomain.UserStatusActive, DeletedAt: &deletedAt},
	}}
	authorizer := &rightsActorAuthorizer{users: reader}
	for _, actorID := range []int64{1, 2, 3} {
		if err := authorizer.AuthorizeRightsRead(context.Background(), sourceapplication.RightsReadAuthorizationDTO{
			ActorID: actorID, Operation: sourceapplication.RightsReadSourceEndpointCapability, SourceEndpointID: 9,
		}); err != nil {
			t.Fatalf("active member %d public capability authorization: %v", actorID, err)
		}
	}
	if err := authorizer.AuthorizeRightsRead(context.Background(), sourceapplication.RightsReadAuthorizationDTO{
		ActorID: 1, Operation: sourceapplication.RightsReadExactActionMatrix, SourceEndpointID: 9,
	}); err != nil {
		t.Fatalf("active administrator exact evaluation authorization: %v", err)
	}
	for _, actorID := range []int64{2, 3, 4, 5, 99} {
		err := authorizer.AuthorizeRightsRead(context.Background(), sourceapplication.RightsReadAuthorizationDTO{
			ActorID: actorID, Operation: sourceapplication.RightsReadDecisionHistory, SourceEndpointID: 9,
		})
		var applicationError *sharederrors.AppError
		if !errors.As(err, &applicationError) || applicationError.Code != sharederrors.CodeForbidden {
			t.Fatalf("actor %d administrator read error = %v, want forbidden", actorID, err)
		}
	}
	for _, invalid := range []sourceapplication.RightsReadAuthorizationDTO{
		{ActorID: 0, Operation: sourceapplication.RightsReadSourceEndpointCapability, SourceEndpointID: 9},
		{ActorID: 1, Operation: sourceapplication.RightsReadSourceEndpointCapability, SourceEndpointID: 0},
		{ActorID: 1, Operation: "invented", SourceEndpointID: 9},
	} {
		if err := authorizer.AuthorizeRightsRead(context.Background(), invalid); !errors.Is(err, sharedrepository.ErrInvalidInput) {
			t.Fatalf("AuthorizeRightsRead(%#v) error = %v, want invalid input", invalid, err)
		}
	}
}

func TestRightsManagementAuditAdapterMapsOnlyBoundedSafeFacts(t *testing.T) {
	writer := &operationsAuditWriterFake{}
	adapter := &rightsManagementAuditAdapter{writer: writer}
	ctx := sharedrequestcontext.WithRequestID(context.Background(), "request-rights-1")
	ctx = sharedrequestcontext.WithTraceID(ctx, strings.Repeat("a", 32))
	sourceID := int64(7)

	policyEvent := sourceapplication.RightsManagementAuditDTO{
		ActorID: 3, Operation: sourceapplication.RightsManagementCreatePolicy, SourceConnectionID: &sourceID,
		PolicyID: 11, IdempotencyKey: "rights.policy.audit", CommandFingerprint: strings.Repeat("b", 64),
	}
	if err := adapter.WriteRightsMutation(ctx, policyEvent); err != nil {
		t.Fatalf("write policy audit: %v", err)
	}
	decisionEvent := sourceapplication.RightsManagementAuditDTO{
		ActorID: 3, Operation: sourceapplication.RightsManagementRecordDecisions, SourceConnectionID: &sourceID,
		PolicyID: 11, DecisionBatchID: 13, DecisionIDs: []int64{17, 19}, Actions: []string{"store_raw", "retain"},
		IdempotencyKey: "rights.decision.audit", CommandFingerprint: strings.Repeat("c", 64),
	}
	if err := adapter.WriteRightsMutation(ctx, decisionEvent); err != nil {
		t.Fatalf("write decision audit: %v", err)
	}
	attemptEvent := sourceapplication.RightsManagementAttemptAuditDTO{
		ActorID: 3, Operation: sourceapplication.RightsManagementRecordDecisions,
		SourceConnectionID: &sourceID, PolicyID: 11,
		Result: sourceapplication.RightsManagementAttemptFailure, ReasonCode: sourceapplication.RightsManagementReasonVersionConflict,
	}
	if err := adapter.WriteRightsMutationAttempt(ctx, attemptEvent); err != nil {
		t.Fatalf("write failed decision attempt audit: %v", err)
	}
	if len(writer.entries) != 3 || writer.independentCalls != 1 {
		t.Fatalf("audit entry count = %d", len(writer.entries))
	}
	if first := writer.entries[0]; first.Action != operationsdomain.ActionRightsPolicyCreated || first.ResourceType != "rights_policy" ||
		first.ResourceID != 11 || first.RequestID != "request-rights-1" || first.TraceID != strings.Repeat("a", 32) || first.After != nil {
		t.Fatalf("policy audit entry = %#v", first)
	}
	if second := writer.entries[1]; second.Action != operationsdomain.ActionRightsDecisionBatchRecorded || second.ResourceType != "rights_decision_batch" ||
		second.ResourceID != 13 || !reflect.DeepEqual(second.After, map[string]any{"decision_count": 2}) {
		t.Fatalf("decision audit entry = %#v", second)
	}
	if attempt := writer.entries[2]; attempt.Action != operationsdomain.ActionRightsDecisionBatchRecorded || attempt.ResourceType != "rights_decision_batch" ||
		attempt.ResourceID != 0 || attempt.Result != operationsdomain.AuditResultFailure ||
		!reflect.DeepEqual(attempt.After, map[string]any{"reason_code": sourceapplication.RightsManagementReasonVersionConflict}) ||
		attempt.IdempotencyKey != "" || attempt.CommandFingerprint != "" {
		t.Fatalf("decision attempt audit entry = %#v", attempt)
	}
	if encoded := strings.ToLower(strings.Join([]string{
		writer.entries[1].RequestID, writer.entries[1].TraceID, writer.entries[1].IdempotencyKey,
		writer.entries[1].CommandFingerprint, writer.entries[1].ResourceType,
	}, " ")); strings.Contains(encoded, "store_raw") || strings.Contains(encoded, "retain") {
		t.Fatalf("audit persisted action/body-like data: %q", encoded)
	}
}

func TestRightsManagementAuditAdapterRejectsIncompleteDecisionReceipt(t *testing.T) {
	writer := &operationsAuditWriterFake{}
	adapter := &rightsManagementAuditAdapter{writer: writer}
	sourceID := int64(7)
	event := sourceapplication.RightsManagementAuditDTO{
		ActorID: 3, Operation: sourceapplication.RightsManagementRecordDecisions, SourceConnectionID: &sourceID,
		PolicyID: 11, DecisionBatchID: 13, DecisionIDs: []int64{17}, Actions: nil,
		IdempotencyKey: "rights.decision.audit", CommandFingerprint: strings.Repeat("c", 64),
	}
	if err := adapter.WriteRightsMutation(context.Background(), event); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("incomplete decision audit error = %v, want invalid input", err)
	}
	if len(writer.entries) != 0 {
		t.Fatal("invalid rights receipt reached Operations audit writer")
	}
}

type rightsActorReaderFake struct {
	users   map[int64]*identitydomain.User
	readIDs []int64
	err     error
}

func (reader *rightsActorReaderFake) FindByID(_ context.Context, id int64) (*identitydomain.User, error) {
	reader.readIDs = append(reader.readIDs, id)
	if reader.err != nil {
		return nil, reader.err
	}
	user, found := reader.users[id]
	if !found {
		return nil, sharedrepository.ErrNotFound
	}
	copy := *user
	return &copy, nil
}

type operationsAuditWriterFake struct {
	entries          []operationsdomain.AuditEntry
	independentCalls int
	err              error
}

func (writer *operationsAuditWriterFake) Write(_ context.Context, entry operationsdomain.AuditEntry) error {
	writer.entries = append(writer.entries, entry)
	return writer.err
}

func (writer *operationsAuditWriterFake) WriteIndependent(_ context.Context, entry operationsdomain.AuditEntry) error {
	writer.independentCalls++
	writer.entries = append(writer.entries, entry)
	return writer.err
}
