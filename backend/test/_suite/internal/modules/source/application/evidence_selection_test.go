package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestEvidenceSelectionServiceReturnsOnlyVerifiedSelectedBytes(t *testing.T) {
	t.Parallel()
	manifest, payload := evidenceSelectionFixture(t)
	manifests := &evidenceSelectionManifestFake{results: []EvidenceSelectionManifestDTO{manifest, manifest}}
	objects := &rawEvidenceObjectReaderFake{payload: payload}
	service, err := NewEvidenceSelectionService(EvidenceSelectionDependencies{
		Manifests: manifests,
		Objects:   objects,
		Selector:  wholePayloadSelectorFake{},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Read(context.Background(), EvidenceSelectionQuery{EvidenceReferenceID: manifest.EvidenceReferenceID})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if manifests.calls != 2 {
		t.Fatalf("manifest reads = %d, want pre/post rights checks", manifests.calls)
	}
	if objects.query.ObjectKey != manifest.ObjectKey || objects.query.MaximumBytes != MaximumRawEvidenceReadBytes {
		t.Fatalf("object query = %#v", objects.query)
	}
	if string(result.Evidence.SelectedPayload) != string(payload) ||
		result.Evidence.SelectedPayloadSHA256 != manifest.EvidenceReference.SelectedPayloadSHA256 ||
		result.Evidence.EvidenceReferenceID != manifest.EvidenceReferenceID ||
		result.Evidence.SourceObservationID != manifest.SourceObservationID {
		t.Fatalf("selected evidence = %#v", result.Evidence)
	}
	payload[0] = 'X'
	if string(result.Evidence.SelectedPayload) == string(payload) {
		t.Fatal("selected evidence aliases object-store bytes")
	}
}

func TestEvidenceSelectionServiceFailsClosedWhenRightsChangeDuringRead(t *testing.T) {
	t.Parallel()
	manifest, payload := evidenceSelectionFixture(t)
	revoked := manifest
	revoked.StoreRawAllowed = false
	manifests := &evidenceSelectionManifestFake{results: []EvidenceSelectionManifestDTO{manifest, revoked}}
	service, err := NewEvidenceSelectionService(EvidenceSelectionDependencies{
		Manifests: manifests,
		Objects:   &rawEvidenceObjectReaderFake{payload: payload},
		Selector:  wholePayloadSelectorFake{},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Read(context.Background(), EvidenceSelectionQuery{EvidenceReferenceID: manifest.EvidenceReferenceID})
	if !errors.Is(err, sharedrepository.ErrConstraint) {
		t.Fatalf("Read() error = %v, want current-rights constraint", err)
	}
	if manifests.calls != 2 {
		t.Fatalf("manifest reads = %d, want post-read revocation check", manifests.calls)
	}
}

func TestEvidenceSelectionServiceRejectsObjectIntegrityMismatch(t *testing.T) {
	t.Parallel()
	manifest, payload := evidenceSelectionFixture(t)
	tampered := append([]byte(nil), payload...)
	tampered[len(tampered)-1] ^= 1
	service, err := NewEvidenceSelectionService(EvidenceSelectionDependencies{
		Manifests: &evidenceSelectionManifestFake{results: []EvidenceSelectionManifestDTO{manifest}},
		Objects:   &rawEvidenceObjectReaderFake{payload: tampered},
		Selector:  wholePayloadSelectorFake{},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.Read(context.Background(), EvidenceSelectionQuery{EvidenceReferenceID: manifest.EvidenceReferenceID})
	if !errors.Is(err, domain.ErrRawEvidenceConflict) {
		t.Fatalf("Read() error = %v, want raw evidence conflict", err)
	}
}

func TestEvidenceSelectionServiceRequiresPOJODependenciesAndValidQuery(t *testing.T) {
	t.Parallel()
	if _, err := NewEvidenceSelectionService(EvidenceSelectionDependencies{}); err == nil {
		t.Fatal("constructor accepted missing ports")
	}
	manifest, payload := evidenceSelectionFixture(t)
	service, err := NewEvidenceSelectionService(EvidenceSelectionDependencies{
		Manifests: &evidenceSelectionManifestFake{results: []EvidenceSelectionManifestDTO{manifest}},
		Objects:   &rawEvidenceObjectReaderFake{payload: payload},
		Selector:  wholePayloadSelectorFake{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Read(context.Background(), EvidenceSelectionQuery{}); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("invalid query error = %v", err)
	}
}

type evidenceSelectionManifestFake struct {
	results []EvidenceSelectionManifestDTO
	calls   int
}

func (reader *evidenceSelectionManifestFake) ReadEvidenceSelectionManifest(_ context.Context, _ int64) (EvidenceSelectionManifestDTO, error) {
	if reader.calls >= len(reader.results) {
		return EvidenceSelectionManifestDTO{}, errors.New("unexpected manifest read")
	}
	result := reader.results[reader.calls]
	reader.calls++
	return result, nil
}

type rawEvidenceObjectReaderFake struct {
	query   ReadRawEvidenceObjectQuery
	payload []byte
}

func (reader *rawEvidenceObjectReaderFake) Read(_ context.Context, query ReadRawEvidenceObjectQuery) (ReadRawEvidenceObjectResult, error) {
	reader.query = query
	return ReadRawEvidenceObjectResult{Payload: append([]byte(nil), reader.payload...)}, nil
}

type wholePayloadSelectorFake struct{}

func (wholePayloadSelectorFake) Select(input EvidenceSelectorInputDTO) ([]byte, error) {
	if err := input.Validate(); err != nil || input.Reference.LocatorType != string(domain.EvidenceLocatorWholePayload) || input.Reference.EvidenceKey != input.Snapshot.EvidenceKey {
		return nil, domain.ErrEvidenceSelection
	}
	return append([]byte(nil), input.Snapshot.Payload...), nil
}

func evidenceSelectionFixture(t *testing.T) (EvidenceSelectionManifestDTO, []byte) {
	t.Helper()
	payload := []byte("<entry><title>semantic monitor</title></entry>")
	digest := sha256.Sum256(payload)
	payloadSHA := hex.EncodeToString(digest[:])
	profile, err := domain.NewCollectorProfileVersion("rss-http-feed-go-xml-v1")
	if err != nil {
		t.Fatal(err)
	}
	evidenceKey, err := domain.EvidenceSnapshotIdentity(payloadSHA, profile)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := NewRawResponseHeadersDTO(map[string][]string{"Content-Type": {"application/xml"}})
	if err != nil {
		t.Fatal(err)
	}
	capturedAt := time.Date(2026, time.August, 9, 10, 0, 0, 0, time.UTC)
	discoveredAt := capturedAt.Add(-time.Minute)
	rightsAt := capturedAt.Add(time.Minute)
	retentionDays := 30
	publishedAt := discoveredAt.Add(-time.Hour)
	manifest := EvidenceSelectionManifestDTO{
		EvidenceReferenceID:     71,
		SourceObservationID:     81,
		EvidenceSnapshotID:      91,
		SourceConnectionID:      101,
		ExternalID:              "entry-1",
		UpstreamIdentity:        payloadSHA,
		SourceCode:              "rss",
		ContentType:             "article",
		Title:                   "Semantic monitor",
		Language:                "zh-CN",
		Author:                  "Publisher",
		SourceRecordURL:         "https://publisher.example.test/feed.xml",
		CanonicalURL:            "https://publisher.example.test/articles/1",
		BodyOrigin:              "feed_content",
		Completeness:            "full",
		PublishedAt:             &publishedAt,
		DiscoveredAt:            discoveredAt,
		ObservationState:        "active",
		LifecycleState:          string(domain.EvidenceLifecycleAvailable),
		EvidenceKey:             evidenceKey,
		ObjectKey:               RawEvidenceObjectKey(101, evidenceKey),
		PayloadSHA256:           payloadSHA,
		CollectorProfileVersion: profile.String(),
		MIMEType:                "application/xml",
		SizeBytes:               int64(len(payload)),
		ResponseStatus:          200,
		RequestedURL:            "https://publisher.example.test/feed.xml",
		FinalURL:                "https://publisher.example.test/feed.xml",
		ResponseHeaders:         headers,
		CapturedAt:              capturedAt,
		RetentionUntil:          capturedAt.Add(30 * 24 * time.Hour),
		StoreRawAllowed:         true,
		RetainAllowed:           true,
		CurrentRetentionDays:    &retentionDays,
		RightsEvaluatedAt:       rightsAt,
		EvidenceReference: RawEvidenceReferenceDTO{
			EvidenceKey: evidenceKey, LocatorType: string(domain.EvidenceLocatorWholePayload), LocatorValue: "/",
			SelectedPayloadSHA256: payloadSHA, SelectorVersion: domain.WholePayloadSelectorVersion,
		},
	}
	return manifest, payload
}
