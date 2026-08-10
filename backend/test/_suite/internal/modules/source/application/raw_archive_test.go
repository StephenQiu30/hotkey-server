package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sharedrequestcontext "github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

const archiveCollectorProfileVersion = "rss-http-feed-go-xml-v1"

type archiveClockFake struct {
	now time.Time
}

func (clock *archiveClockFake) Now() time.Time { return clock.now }

type rawEvidenceStoreFake struct {
	objects map[string]StoreRawEvidenceCommand
	calls   int
	err     error
}

func (store *rawEvidenceStoreFake) PutIfAbsent(_ context.Context, command StoreRawEvidenceCommand) (StoreRawEvidenceResult, error) {
	store.calls++
	if store.err != nil {
		return StoreRawEvidenceResult{}, store.err
	}
	if err := command.Validate(); err != nil {
		return StoreRawEvidenceResult{}, err
	}
	if store.objects == nil {
		store.objects = make(map[string]StoreRawEvidenceCommand)
	}
	if existing, found := store.objects[command.ObjectKey]; found {
		if existing.SourceConnectionID != command.SourceConnectionID || existing.EvidenceKey != command.EvidenceKey ||
			existing.PayloadSHA256 != command.PayloadSHA256 || existing.CollectorProfileVersion != command.CollectorProfileVersion ||
			existing.MIMEType != command.MIMEType || !bytes.Equal(existing.Payload, command.Payload) {
			return StoreRawEvidenceResult{}, domain.ErrRawEvidenceConflict
		}
		return rawEvidenceStoreResult(existing), nil
	}
	copy := command
	copy.Payload = append([]byte(nil), command.Payload...)
	store.objects[command.ObjectKey] = copy
	return rawEvidenceStoreResult(copy), nil
}

func rawEvidenceStoreResult(command StoreRawEvidenceCommand) StoreRawEvidenceResult {
	return StoreRawEvidenceResult{
		SourceConnectionID: command.SourceConnectionID, EvidenceKey: command.EvidenceKey, ObjectKey: command.ObjectKey,
		PayloadSHA256: command.PayloadSHA256, CollectorProfileVersion: command.CollectorProfileVersion,
		MIMEType: command.MIMEType, SizeBytes: int64(len(command.Payload)),
	}
}

type evidenceSnapshotRepositoryFake struct {
	reservations []ReserveEvidenceSnapshotCommand
	records      map[string]PersistedEvidenceSnapshotDTO
	commits      []CommitEvidenceSnapshotCommand
	failed       []int64
}

func (repository *evidenceSnapshotRepositoryFake) Reserve(_ context.Context, command ReserveEvidenceSnapshotCommand) (PersistedEvidenceSnapshotDTO, error) {
	repository.reservations = append(repository.reservations, command)
	if repository.records == nil {
		repository.records = make(map[string]PersistedEvidenceSnapshotDTO)
	}
	identity := fmt.Sprintf("%d:%s", command.SourceConnectionID, command.EvidenceKey)
	if existing, found := repository.records[identity]; found {
		if existing.PayloadSHA256 != command.PayloadSHA256 || existing.CollectorProfileVersion != command.CollectorProfileVersion ||
			existing.ObjectKey != command.ObjectKey || existing.SizeBytes != command.SizeBytes {
			return PersistedEvidenceSnapshotDTO{}, domain.ErrRawEvidenceConflict
		}
		return existing, nil
	}
	record := persistedEvidenceSnapshotFromReservation(int64(len(repository.records)+101), command)
	repository.records[identity] = record
	return record, nil
}

func (repository *evidenceSnapshotRepositoryFake) Commit(_ context.Context, command CommitEvidenceSnapshotCommand) (CommitEvidenceSnapshotResult, error) {
	repository.commits = append(repository.commits, command)
	for identity, record := range repository.records {
		if record.ID == command.SnapshotID {
			record.LifecycleState = string(domain.EvidenceLifecycleAvailable)
			repository.records[identity] = record
			references := make([]CommittedEvidenceReferenceDTO, len(command.Observations))
			for index := range command.Observations {
				references[index] = CommittedEvidenceReferenceDTO{
					EvidenceReferenceID: record.ID*100 + int64(index+1),
					SourceObservationID: int64(index + 1), EvidenceSnapshotID: record.ID,
					Usage: command.Observations[index].Evidence.Usage,
				}
			}
			return CommitEvidenceSnapshotResult{Snapshot: record, EvidenceReferences: references}, nil
		}
	}
	return CommitEvidenceSnapshotResult{}, errors.New("evidence snapshot was not reserved")
}

func (repository *evidenceSnapshotRepositoryFake) MarkFailed(_ context.Context, snapshotID int64, _ string) error {
	repository.failed = append(repository.failed, snapshotID)
	return nil
}

func persistedEvidenceSnapshotFromReservation(id int64, command ReserveEvidenceSnapshotCommand) PersistedEvidenceSnapshotDTO {
	return PersistedEvidenceSnapshotDTO{
		ID: id, LifecycleState: string(domain.EvidenceLifecyclePending),
		SourceConnectionID: command.SourceConnectionID, CollectionRunID: command.CollectionRunID,
		StoreRawRightsDecisionID: command.StoreRawRightsDecisionID, RetainRightsDecisionID: command.RetainRightsDecisionID,
		EvidenceKey: command.EvidenceKey, ObjectKey: command.ObjectKey, PayloadSHA256: command.PayloadSHA256,
		CollectorProfileVersion: command.CollectorProfileVersion, MIMEType: command.MIMEType, SizeBytes: command.SizeBytes,
		ResponseStatus: command.ResponseStatus, RequestedURL: command.RequestedURL, FinalURL: command.FinalURL,
		RedirectChain: append([]string(nil), command.RedirectChain...), ResponseHeaders: command.ResponseHeaders,
		CapturedAt: command.CapturedAt, RetentionUntil: command.RetentionUntil,
	}
}

func TestRawEvidenceArchiveVerifiesAndPersistsSourceOwnedEvidence(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 9, 6, 0, 0, 0, time.UTC)
	payload := []byte("<feed><entry>one</entry><entry>two</entry></feed>")
	snapshot := archiveSnapshot(t, now, payload, archiveCollectorProfileVersion)
	first := byteRangeItem(snapshot, "entry-1", []byte("<entry>one</entry>"))
	second := byteRangeItem(snapshot, "entry-2", []byte("<entry>two</entry>"))
	bodyMarker := "synchronous-body-marker-must-not-persist"
	first.Body = bodyMarker
	bodyVariant := first
	bodyVariant.Body = "different-transient-body"
	if sourceObservationIdentity(first) != sourceObservationIdentity(bodyVariant) {
		t.Fatal("transient Body influenced the durable source observation identity")
	}
	store := &rawEvidenceStoreFake{}
	repository := &evidenceSnapshotRepositoryFake{}
	service := newArchiveService(t, store, repository, &archiveClockFake{now: now})

	traceID := "0123456789abcdef0123456789abcdef"
	ctx := sharedrequestcontext.WithTraceID(context.Background(), traceID)
	result, err := service.Archive(ctx, archiveCommand(snapshot, []domain.SourceItem{first, second}, now))
	if err != nil {
		t.Fatalf("Archive(): %v", err)
	}
	if len(result.Snapshots) != 1 || result.Snapshots[0].ID != 101 || result.Snapshots[0].LifecycleState != string(domain.EvidenceLifecycleAvailable) {
		t.Fatalf("Archive() = %#v, want one available evidence snapshot", result)
	}
	if store.calls != 1 || len(store.objects) != 1 || len(repository.reservations) != 1 || len(repository.commits) != 1 {
		t.Fatalf("store/repository calls = %d/%d/%d/%d", store.calls, len(store.objects), len(repository.reservations), len(repository.commits))
	}
	reservation := repository.reservations[0]
	if reservation.StoreRawRightsDecisionID != 13 || reservation.RetainRightsDecisionID != 14 ||
		reservation.CollectorProfileVersion != archiveCollectorProfileVersion || reservation.EvidenceKey != snapshot.Key ||
		reservation.ObjectKey != RawEvidenceObjectKey(42, snapshot.Key) {
		t.Fatalf("reservation = %#v", reservation)
	}
	if len(repository.commits[0].Observations) != 2 || repository.commits[0].Observations[0].Evidence.LocatorType != string(domain.EvidenceLocatorByteRange) {
		t.Fatalf("commit = %#v, want independently verified observations", repository.commits[0])
	}
	if repository.commits[0].TraceID != traceID || repository.commits[0].DocumentGenerationScheduledAt != now {
		t.Fatalf("commit scheduling context = trace:%q at:%s", repository.commits[0].TraceID, repository.commits[0].DocumentGenerationScheduledAt)
	}
	for _, forbidden := range []string{"Authorization", "Cookie", "Set-Cookie", "X-Upstream-Secret"} {
		if _, found := reservation.ResponseHeaders.Values()[forbidden]; found {
			t.Fatalf("reservation retained forbidden response header %q", forbidden)
		}
	}
	if strings.Contains(fmt.Sprintf("%#v", repository.reservations), bodyMarker) || strings.Contains(fmt.Sprintf("%#v", repository.commits), bodyMarker) {
		t.Fatal("synchronous RawEvidenceItemDTO.Body flowed into a repository command")
	}
}

func TestRawEvidenceArchiveAcceptsConcreteEndpointAuthorizationForExactSnapshot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 9, 6, 0, 0, 0, time.UTC)
	snapshot := archiveSnapshot(t, now, []byte("<feed><entry>endpoint</entry></feed>"), archiveCollectorProfileVersion)
	item := byteRangeItem(snapshot, "endpoint-entry", []byte("<entry>endpoint</entry>"))
	command := archiveCommand(snapshot, []domain.SourceItem{item}, now)
	expiresAt := now.Add(24 * time.Hour)
	retentionDays := 30
	command.StoreRawDecisions[snapshot.Key] = RawEvidenceRightsDecisionDTO{
		ID: 31, PolicyID: 41, PolicyRevision: 1, SourceConnectionID: command.SourceConnectionID,
		SubjectType: RawEvidenceRightsSubjectEndpoint, SubjectKey: "42", InputDigest: strings.Repeat("a", 64),
		AuthorizedEvidenceKey: snapshot.Key, AuthorizedPayloadSHA256: snapshot.PayloadSHA256,
		Action: string(domain.RightsActionStoreRaw), Decision: string(domain.RightsAllow),
		EffectiveFrom: now.Add(-time.Hour), ExpiresAt: &expiresAt,
	}
	command.RetainDecisions[snapshot.Key] = RawEvidenceRightsDecisionDTO{
		ID: 32, PolicyID: 41, PolicyRevision: 1, SourceConnectionID: command.SourceConnectionID,
		SubjectType: RawEvidenceRightsSubjectEndpoint, SubjectKey: "42", InputDigest: strings.Repeat("a", 64),
		AuthorizedEvidenceKey: snapshot.Key, AuthorizedPayloadSHA256: snapshot.PayloadSHA256,
		Action: string(domain.RightsActionRetain), Decision: string(domain.RightsAllow), RetentionDays: &retentionDays,
		EffectiveFrom: now.Add(-time.Hour), ExpiresAt: &expiresAt,
	}
	store := &rawEvidenceStoreFake{}
	repository := &evidenceSnapshotRepositoryFake{}
	service := newArchiveService(t, store, repository, &archiveClockFake{now: now})

	if _, err := service.Archive(context.Background(), command); err != nil {
		t.Fatalf("Archive() endpoint authorization error = %v", err)
	}
	if len(repository.reservations) != 1 || repository.reservations[0].StoreRawRightsDecisionID != 31 ||
		repository.reservations[0].RetainRightsDecisionID != 32 {
		t.Fatalf("endpoint-authorized reservation = %#v", repository.reservations)
	}

	tampered := command
	tampered.StoreRawDecisions = map[string]RawEvidenceRightsDecisionDTO{snapshot.Key: command.StoreRawDecisions[snapshot.Key]}
	invalid := tampered.StoreRawDecisions[snapshot.Key]
	invalid.AuthorizedPayloadSHA256 = strings.Repeat("b", 64)
	tampered.StoreRawDecisions[snapshot.Key] = invalid
	if _, err := service.Archive(context.Background(), tampered); err == nil {
		t.Fatal("Archive() accepted endpoint authorization for another payload")
	}
}

func TestRawEvidenceArchiveFailsClosedOnInvalidSelectorBeforeExternalCalls(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 9, 6, 0, 0, 0, time.UTC)
	snapshot := archiveSnapshot(t, now, []byte("<feed><entry>one</entry></feed>"), archiveCollectorProfileVersion)
	validItem := byteRangeItem(snapshot, "entry-1", []byte("<entry>one</entry>"))
	tests := []struct {
		name   string
		mutate func(*domain.EvidenceReference)
	}{
		{name: "selected hash does not match bytes", mutate: func(reference *domain.EvidenceReference) {
			reference.SelectedPayloadSHA256 = fmt.Sprintf("%064d", 0)
		}},
		{name: "byte range exceeds payload", mutate: func(reference *domain.EvidenceReference) {
			end := int64(len(snapshot.Payload) + 1)
			reference.ByteEnd = &end
			reference.LocatorValue = fmt.Sprintf("bytes[%d:%d]", *reference.ByteStart, end)
		}},
		{name: "unknown locator", mutate: func(reference *domain.EvidenceReference) {
			reference.LocatorType = domain.EvidenceLocatorJSONPointer
			reference.LocatorValue = "/feed/entry/0"
			reference.ByteStart, reference.ByteEnd = nil, nil
			reference.SelectorVersion = "json-pointer-sha256-v1"
		}},
		{name: "unknown XML selector version", mutate: func(reference *domain.EvidenceReference) {
			reference.LocatorType = domain.EvidenceLocatorXMLPath
			reference.LocatorValue = "/feed/entry[1]"
			reference.ByteStart, reference.ByteEnd = nil, nil
			reference.SelectorVersion = "atom-unknown-v1"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := validItem
			item.EvidenceReferences = append([]domain.EvidenceReference(nil), validItem.EvidenceReferences...)
			test.mutate(&item.EvidenceReferences[0])
			item.SnapshotKey = item.EvidenceReferences[0].SnapshotKey
			item.ItemLocator = item.EvidenceReferences[0].LocatorValue
			store := &rawEvidenceStoreFake{}
			repository := &evidenceSnapshotRepositoryFake{}
			service := newArchiveService(t, store, repository, &archiveClockFake{now: now})
			_, err := service.Archive(context.Background(), archiveCommand(snapshot, []domain.SourceItem{item}, now))
			if !errors.Is(err, domain.ErrEvidenceSelection) {
				t.Fatalf("Archive() error = %v, want evidence selection failure", err)
			}
			if store.calls != 0 || len(repository.reservations) != 0 || len(repository.commits) != 0 {
				t.Fatalf("invalid selector reached external ports: %#v / %#v", store, repository)
			}
		})
	}
}

func TestRawEvidenceArchiveRejectsTamperedDigestBeforeExternalCalls(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 9, 6, 0, 0, 0, time.UTC)
	snapshot := archiveSnapshot(t, now, []byte("<feed/>"), archiveCollectorProfileVersion)
	tampered := snapshot
	tampered.PayloadSHA256 = fmt.Sprintf("%064d", 0)
	store := &rawEvidenceStoreFake{}
	repository := &evidenceSnapshotRepositoryFake{}
	service := newArchiveService(t, store, repository, &archiveClockFake{now: now})
	_, err := service.Archive(context.Background(), archiveCommand(tampered, nil, now))
	if !errors.Is(err, domain.ErrRawEvidenceConflict) {
		t.Fatalf("Archive() error = %v, want evidence conflict", err)
	}
	if store.calls != 0 || len(repository.reservations) != 0 {
		t.Fatalf("tampered digest reached external ports: %#v / %#v", store, repository)
	}
}

func TestRawEvidenceArchiveEnforcesCaptureClockAndLiveRetentionBeforeExternalCalls(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 9, 6, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		capturedAt time.Time
		wantError  bool
	}{
		{name: "maximum clock skew is accepted", capturedAt: now.Add(MaximumCaptureClockSkew)},
		{name: "future capture exceeds clock skew", capturedAt: now.Add(MaximumCaptureClockSkew + time.Nanosecond), wantError: true},
		{name: "old response retention already elapsed", capturedAt: now.Add(-31 * 24 * time.Hour), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := archiveSnapshot(t, test.capturedAt, []byte("<feed/>"), archiveCollectorProfileVersion)
			store := &rawEvidenceStoreFake{}
			repository := &evidenceSnapshotRepositoryFake{}
			service := newArchiveService(t, store, repository, &archiveClockFake{now: now})
			_, err := service.Archive(context.Background(), archiveCommand(snapshot, nil, now))
			if test.wantError {
				if err == nil {
					t.Fatal("Archive() accepted invalid capture/retention time")
				}
				if store.calls != 0 || len(repository.reservations) != 0 {
					t.Fatalf("invalid time reached external ports: %#v / %#v", store, repository)
				}
				return
			}
			if err != nil {
				t.Fatalf("Archive() rejected maximum allowed skew: %v", err)
			}
		})
	}
}

func TestRawEvidenceArchiveRecapturePreservesFirstManifestFacts(t *testing.T) {
	t.Parallel()

	firstNow := time.Date(2026, time.August, 9, 6, 0, 0, 0, time.UTC)
	clock := &archiveClockFake{now: firstNow}
	payload := []byte("<feed><entry>same</entry></feed>")
	firstSnapshot := archiveSnapshot(t, firstNow, payload, archiveCollectorProfileVersion)
	store := &rawEvidenceStoreFake{}
	repository := &evidenceSnapshotRepositoryFake{}
	service := newArchiveService(t, store, repository, clock)
	firstCommand := archiveCommand(firstSnapshot, []domain.SourceItem{byteRangeItem(firstSnapshot, "entry", []byte("<entry>same</entry>"))}, firstNow)
	firstResult, err := service.Archive(context.Background(), firstCommand)
	if err != nil {
		t.Fatal(err)
	}
	firstFacts := firstResult.Snapshots[0]

	secondNow := firstNow.Add(24 * time.Hour)
	clock.now = secondNow
	profile, err := domain.NewCollectorProfileVersion(archiveCollectorProfileVersion)
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot, err := domain.NewEvidenceSnapshot(domain.EvidenceSnapshot{
		Payload: payload, CollectorProfileVersion: profile, MIMEType: "application/xml", StatusCode: 206,
		RequestedURL: "https://feed.example.test/changed.xml", FinalURL: "https://feed.example.test/changed.xml", CapturedAt: secondNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if secondSnapshot.Key != firstSnapshot.Key {
		t.Fatal("same payload/profile recapture changed evidence identity")
	}
	secondCommand := archiveCommand(secondSnapshot, []domain.SourceItem{byteRangeItem(secondSnapshot, "entry", []byte("<entry>same</entry>"))}, secondNow)
	secondCommand.CollectionRunID = 8
	retentionDays := 90
	secondRetain := secondCommand.RetainDecisions[secondSnapshot.Key]
	secondRetain.RetentionDays = &retentionDays
	secondCommand.RetainDecisions[secondSnapshot.Key] = secondRetain
	secondResult, err := service.Archive(context.Background(), secondCommand)
	if err != nil {
		t.Fatal(err)
	}
	secondFacts := secondResult.Snapshots[0]
	if !samePersistedEvidenceFacts(firstFacts, secondFacts) || !secondFacts.CapturedAt.Equal(firstSnapshot.CapturedAt) ||
		!secondFacts.RetentionUntil.Equal(firstFacts.RetentionUntil) || secondFacts.CollectionRunID != firstCommand.CollectionRunID ||
		secondFacts.MIMEType != firstSnapshot.MIMEType {
		t.Fatalf("recapture overwrote first facts: first=%#v second=%#v", firstFacts, secondFacts)
	}
	if len(store.objects) != 1 || len(repository.records) != 1 || store.objects[firstFacts.ObjectKey].MIMEType != firstSnapshot.MIMEType {
		t.Fatalf("recapture changed immutable object: store=%#v repository=%#v", store, repository)
	}
}

func TestRawEvidenceArchivePreflightsAuthorizationAndMarksPendingStoreFailure(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 9, 6, 0, 0, 0, time.UTC)
	snapshot := archiveSnapshot(t, now, []byte("<feed/>"), archiveCollectorProfileVersion)
	command := archiveCommand(snapshot, nil, now)
	denied := command.StoreRawDecisions[snapshot.Key]
	denied.Decision = string(domain.RightsDeny)
	command.StoreRawDecisions[snapshot.Key] = denied
	store := &rawEvidenceStoreFake{}
	repository := &evidenceSnapshotRepositoryFake{}
	service := newArchiveService(t, store, repository, &archiveClockFake{now: now})
	if _, err := service.Archive(context.Background(), command); !errors.Is(err, domain.ErrRawArchiveNotAuthorized) {
		t.Fatalf("Archive() error = %v, want authorization failure", err)
	}
	if store.calls != 0 || len(repository.reservations) != 0 {
		t.Fatalf("unauthorized archive reached external ports: %#v / %#v", store, repository)
	}

	store.err = errors.New("object store unavailable")
	command.StoreRawDecisions[snapshot.Key] = rawStoreAllowDecision(snapshot, now)
	if _, err := service.Archive(context.Background(), command); err == nil || len(repository.failed) != 1 || repository.failed[0] != 101 {
		t.Fatalf("store failure result = %v, failed=%#v", err, repository.failed)
	}
}

func newArchiveService(t *testing.T, store RawEvidenceStore, repository EvidenceSnapshotRepository, clock Clock) *RawEvidenceArchiveService {
	t.Helper()
	service, err := NewRawEvidenceArchiveService(RawEvidenceArchiveServiceDependencies{
		Store: store, Repository: repository, SelectorVerifier: rawEvidenceSelectorVerifierFake{},
		Clock: clock, MaxCaptureClockSkew: MaximumCaptureClockSkew,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func archiveSnapshot(t *testing.T, capturedAt time.Time, payload []byte, profileValue string) domain.EvidenceSnapshot {
	t.Helper()
	profile, err := domain.NewCollectorProfileVersion(profileValue)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := domain.NewRawResponseHeaders(map[string][]string{
		"Content-Type": {"application/atom+xml; charset=utf-8"}, "ETag": {`"feed-v1"`},
		"Authorization": {"Bearer must-not-survive"}, "Cookie": {"session=must-not-survive"},
		"Set-Cookie": {"upstream=must-not-survive"}, "X-Upstream-Secret": {"must-not-survive"},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := domain.NewEvidenceSnapshot(domain.EvidenceSnapshot{
		Payload: payload, CollectorProfileVersion: profile, MIMEType: "application/atom+xml; charset=utf-8", StatusCode: 200,
		RequestedURL: "https://feed.example.test/source.xml", FinalURL: "https://feed.example.test/source.xml",
		CapturedAt: capturedAt, ResponseHeaders: headers,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func byteRangeItem(snapshot domain.EvidenceSnapshot, externalID string, selected []byte) domain.SourceItem {
	startIndex := bytes.Index(snapshot.Payload, selected)
	start, end := int64(startIndex), int64(startIndex+len(selected))
	digest := sha256.Sum256(selected)
	return domain.SourceItem{
		SourceCode: "rss", ExternalID: externalID, ContentType: "article", Title: externalID,
		Body: string(selected), Language: "en", URL: "https://publisher.example.test/" + externalID,
		ObservedAt: snapshot.CapturedAt, EvidenceCompleteness: domain.EvidenceCompletenessFullBody,
		EvidenceReferences: []domain.EvidenceReference{{
			SnapshotKey: snapshot.Key, LocatorType: domain.EvidenceLocatorByteRange,
			LocatorValue: fmt.Sprintf("bytes[%d:%d]", start, end), ByteStart: &start, ByteEnd: &end,
			SelectedPayloadSHA256: fmt.Sprintf("%x", digest), SelectorVersion: domain.ByteRangeSelectorVersion,
		}},
	}
}

func archiveCommand(snapshot domain.EvidenceSnapshot, items []domain.SourceItem, at time.Time) ArchiveRawEvidenceCommand {
	return ArchiveRawEvidenceCommand{
		SourceConnectionID: 42, CollectionRunID: 7,
		Fetch:             rawEvidenceFetchDTOFromEntity(domain.FetchResult{Snapshots: []domain.EvidenceSnapshot{snapshot}, Items: items}),
		StoreRawDecisions: map[string]RawEvidenceRightsDecisionDTO{snapshot.Key: rawStoreAllowDecision(snapshot, at)},
		RetainDecisions:   map[string]RawEvidenceRightsDecisionDTO{snapshot.Key: rawRetainAllowDecision(snapshot, at)},
	}
}

func rawStoreAllowDecision(snapshot domain.EvidenceSnapshot, at time.Time) RawEvidenceRightsDecisionDTO {
	expiresAt := at.Add(24 * time.Hour)
	return RawEvidenceRightsDecisionDTO{
		ID: 13, PolicyID: 17, PolicyRevision: 2, SourceConnectionID: 42,
		SubjectType: RawEvidenceRightsSubjectResponse, SubjectKey: snapshot.Key, InputDigest: snapshot.PayloadSHA256,
		Action: string(domain.RightsActionStoreRaw), Decision: string(domain.RightsAllow),
		EffectiveFrom: at.Add(-time.Hour), ExpiresAt: &expiresAt,
	}
}

func rawRetainAllowDecision(snapshot domain.EvidenceSnapshot, at time.Time) RawEvidenceRightsDecisionDTO {
	expiresAt := at.Add(24 * time.Hour)
	retentionDays := 30
	return RawEvidenceRightsDecisionDTO{
		ID: 14, PolicyID: 17, PolicyRevision: 2, SourceConnectionID: 42,
		SubjectType: RawEvidenceRightsSubjectResponse, SubjectKey: snapshot.Key, InputDigest: snapshot.PayloadSHA256,
		Action: string(domain.RightsActionRetain), Decision: string(domain.RightsAllow),
		EffectiveFrom: at.Add(-time.Hour), ExpiresAt: &expiresAt, RetentionDays: &retentionDays,
	}
}

type rawEvidenceSelectorVerifierFake struct{}

func (rawEvidenceSelectorVerifierFake) Verify(input EvidenceSelectorInputDTO) error {
	if err := input.Validate(); err != nil {
		return err
	}
	reference, payload := input.Reference, input.Snapshot.Payload
	var selected []byte
	switch reference.LocatorType {
	case "whole_payload":
		if reference.SelectorVersion != "whole-payload-sha256-v1" || reference.LocatorValue != "/" {
			return errors.New("unsupported whole-payload selector")
		}
		selected = payload
	case "byte_range":
		if reference.SelectorVersion != "byte-range-sha256-v1" || reference.ByteStart == nil || reference.ByteEnd == nil ||
			*reference.ByteStart < 0 || *reference.ByteEnd > int64(len(payload)) || *reference.ByteEnd <= *reference.ByteStart ||
			reference.LocatorValue != fmt.Sprintf("bytes[%d:%d]", *reference.ByteStart, *reference.ByteEnd) {
			return errors.New("unsupported byte-range selector")
		}
		selected = payload[*reference.ByteStart:*reference.ByteEnd]
	default:
		return errors.New("unsupported selector")
	}
	digest := sha256.Sum256(selected)
	if fmt.Sprintf("%x", digest) != reference.SelectedPayloadSHA256 {
		return errors.New("selected evidence digest mismatch")
	}
	return nil
}
