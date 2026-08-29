package application_test

import (
	"context"
	"testing"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
)

func TestCollectionAdmissionRequiresAnExplicitCurrentFetchAllow(t *testing.T) {
	at := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	connection := collectionAdmissionConnection()

	for _, test := range []struct {
		name       string
		result     sourceapplication.CurrentCollectionFetchRightsResult
		wantPassed bool
	}{
		{
			name: "explicit allow",
			result: sourceapplication.CurrentCollectionFetchRightsResult{
				Decision: domain.RightsAllow, DecisionIDs: []int64{41}, PolicyIDs: []int64{31}, EvaluatedAt: at,
			},
			wantPassed: true,
		},
		{name: "missing decision is unknown", result: sourceapplication.CurrentCollectionFetchRightsResult{Decision: domain.RightsUnknown, EvaluatedAt: at}},
		{name: "explicit deny", result: sourceapplication.CurrentCollectionFetchRightsResult{Decision: domain.RightsDeny, DecisionIDs: []int64{42}, PolicyIDs: []int64{32}, EvaluatedAt: at}},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &collectionFetchRightsReaderFake{result: test.result}
			credentials := &collectionCredentialStatusFake{available: true}
			budget := &collectionBudgetStatusFake{available: true}
			gate, err := sourceapplication.NewCollectionAdmissionGate(sourceapplication.CollectionAdmissionDependencies{
				Rights:      reader,
				Credentials: credentials,
				Budget:      budget,
				Clock:       collectionAdmissionClock{at: at},
			})
			if err != nil {
				t.Fatalf("NewCollectionAdmissionGate(): %v", err)
			}
			err = gate.AuthorizeCollection(context.Background(), connection)
			if test.wantPassed && err != nil {
				t.Fatalf("AuthorizeCollection() error = %v", err)
			}
			if !test.wantPassed && (err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent) {
				t.Fatalf("AuthorizeCollection() error = %v, want safe permanent rejection", err)
			}
			if reader.query.SourceConnectionID != connection.ID || !reader.query.DecisionAt.Equal(at) {
				t.Fatalf("rights query = %#v", reader.query)
			}
		})
	}
}

func TestCollectionAdmissionRejectsInvalidCapabilityBeforeRightsLookup(t *testing.T) {
	at := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	reader := &collectionFetchRightsReaderFake{}
	gate, err := sourceapplication.NewCollectionAdmissionGate(sourceapplication.CollectionAdmissionDependencies{
		Rights:      reader,
		Credentials: &collectionCredentialStatusFake{available: true},
		Budget:      &collectionBudgetStatusFake{available: true},
		Clock:       collectionAdmissionClock{at: at},
	})
	if err != nil {
		t.Fatal(err)
	}
	connection := collectionAdmissionConnection()
	connection.SourceType = domain.SourceType("unsupported")
	if err := gate.AuthorizeCollection(context.Background(), connection); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorPermanent {
		t.Fatalf("invalid capability error = %v", err)
	}
	if reader.calls != 0 {
		t.Fatalf("rights reader calls = %d, want 0 before capability validation", reader.calls)
	}
}

func TestCollectionAdmissionChecksManagedCredentialStatusAfterRights(t *testing.T) {
	at := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	order := []string{}
	rights := &collectionFetchRightsReaderFake{
		result: sourceapplication.CurrentCollectionFetchRightsResult{
			Decision: domain.RightsAllow, DecisionIDs: []int64{41}, PolicyIDs: []int64{31}, EvaluatedAt: at,
		},
		order: &order,
	}
	credentials := &collectionCredentialStatusFake{available: false, order: &order}
	budget := &collectionBudgetStatusFake{available: true, order: &order}
	gate, err := sourceapplication.NewCollectionAdmissionGate(sourceapplication.CollectionAdmissionDependencies{
		Rights: rights, Credentials: credentials, Budget: budget, Clock: collectionAdmissionClock{at: at},
	})
	if err != nil {
		t.Fatal(err)
	}
	connection := collectionAdmissionConnection()
	connection.SourceType = domain.SourceTypeX
	connection.Endpoint = domain.XRecentSearchEndpoint
	connection.AuthType = domain.AuthTypeBearer
	connection.CredentialRef = domain.ManagedCredentialReference
	if err := gate.AuthorizeCollection(context.Background(), connection); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorAuthentication {
		t.Fatalf("missing managed credential status error = %v", err)
	}
	if credentials.sourceID != connection.ID || len(order) != 2 || order[0] != "rights" || order[1] != "credential_status" {
		t.Fatalf("admission order/status source = %#v/%d", order, credentials.sourceID)
	}
	if budget.calls != 0 {
		t.Fatalf("budget status calls after missing credential = %d, want 0", budget.calls)
	}
}

func TestCollectionAdmissionChecksBudgetAndRateLimitAfterCredentialStatus(t *testing.T) {
	at := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	order := []string{}
	rights := &collectionFetchRightsReaderFake{
		result: sourceapplication.CurrentCollectionFetchRightsResult{
			Decision: domain.RightsAllow, DecisionIDs: []int64{41}, PolicyIDs: []int64{31}, EvaluatedAt: at,
		},
		order: &order,
	}
	credentials := &collectionCredentialStatusFake{available: true, order: &order}
	budget := &collectionBudgetStatusFake{available: false, order: &order}
	gate, err := sourceapplication.NewCollectionAdmissionGate(sourceapplication.CollectionAdmissionDependencies{
		Rights: rights, Credentials: credentials, Budget: budget, Clock: collectionAdmissionClock{at: at},
	})
	if err != nil {
		t.Fatal(err)
	}
	connection := collectionAdmissionConnection()
	connection.SourceType = domain.SourceTypeX
	connection.Endpoint = domain.XRecentSearchEndpoint
	connection.AuthType = domain.AuthTypeBearer
	connection.CredentialRef = domain.ManagedCredentialReference
	if err := gate.AuthorizeCollection(context.Background(), connection); err == nil || domain.ClassifyCollectionError(err) != domain.CollectionErrorRateLimited {
		t.Fatalf("exhausted budget/rate status error = %v", err)
	}
	if budget.sourceID != connection.ID || !budget.at.Equal(at) || len(order) != 3 || order[0] != "rights" || order[1] != "credential_status" || order[2] != "budget_rate_status" {
		t.Fatalf("admission order/budget source/time = %#v/%d/%s", order, budget.sourceID, budget.at)
	}
}

func TestCurrentCollectionFetchRightsResultRejectsDuplicateReceipts(t *testing.T) {
	decisionAt := time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)
	query := sourceapplication.CurrentCollectionFetchRightsQuery{SourceConnectionID: 22, DecisionAt: decisionAt}
	result := sourceapplication.CurrentCollectionFetchRightsResult{
		Decision: domain.RightsAllow, DecisionIDs: []int64{7, 7}, PolicyIDs: []int64{9}, EvaluatedAt: decisionAt,
	}
	if err := result.Validate(query); err == nil {
		t.Fatal("duplicate decision receipts were accepted")
	}
}

type allowingCollectionAdmission struct{}

func (allowingCollectionAdmission) AuthorizeCollection(context.Context, domain.SourceConnection) error {
	return nil
}

func newCollectionServiceForTest(dependencies sourceapplication.CollectionDependencies) (*sourceapplication.CollectionService, error) {
	if dependencies.Admission == nil {
		dependencies.Admission = allowingCollectionAdmission{}
	}
	return sourceapplication.NewCollectionService(dependencies)
}

type collectionFetchRightsReaderFake struct {
	query  sourceapplication.CurrentCollectionFetchRightsQuery
	result sourceapplication.CurrentCollectionFetchRightsResult
	calls  int
	order  *[]string
}

func (reader *collectionFetchRightsReaderFake) ResolveCurrentFetch(_ context.Context, query sourceapplication.CurrentCollectionFetchRightsQuery) (sourceapplication.CurrentCollectionFetchRightsResult, error) {
	reader.calls++
	if reader.order != nil {
		*reader.order = append(*reader.order, "rights")
	}
	reader.query = query
	return reader.result, nil
}

type collectionCredentialStatusFake struct {
	available bool
	sourceID  int64
	order     *[]string
}

type collectionBudgetStatusFake struct {
	available bool
	sourceID  int64
	at        time.Time
	calls     int
	order     *[]string
}

func (status *collectionBudgetStatusFake) CollectionRequestAvailable(_ context.Context, connection domain.SourceConnection, at time.Time) (bool, error) {
	status.sourceID = connection.ID
	status.at = at
	status.calls++
	if status.order != nil {
		*status.order = append(*status.order, "budget_rate_status")
	}
	return status.available, nil
}

func (status *collectionCredentialStatusFake) ManagedCredentialAvailable(_ context.Context, sourceID int64) (bool, error) {
	status.sourceID = sourceID
	if status.order != nil {
		*status.order = append(*status.order, "credential_status")
	}
	return status.available, nil
}

type collectionAdmissionClock struct{ at time.Time }

func (clock collectionAdmissionClock) Now() time.Time { return clock.at }

func collectionAdmissionConnection() domain.SourceConnection {
	return domain.SourceConnection{
		ID: 17, Version: 2, SourceType: domain.SourceTypeRSS, Name: "Admission RSS",
		Endpoint: "https://feeds.example.test/admission", AuthType: domain.AuthTypeNone,
		Config: domain.DefaultSourceConfig(), Enabled: true, HealthStatus: domain.HealthStatusHealthy,
	}
}
