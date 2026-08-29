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
			gate, err := sourceapplication.NewCollectionAdmissionGate(sourceapplication.CollectionAdmissionDependencies{
				Rights: reader,
				Clock:  collectionAdmissionClock{at: at},
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
		Rights: reader,
		Clock:  collectionAdmissionClock{at: at},
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
}

func (reader *collectionFetchRightsReaderFake) ResolveCurrentFetch(_ context.Context, query sourceapplication.CurrentCollectionFetchRightsQuery) (sourceapplication.CurrentCollectionFetchRightsResult, error) {
	reader.calls++
	reader.query = query
	return reader.result, nil
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
