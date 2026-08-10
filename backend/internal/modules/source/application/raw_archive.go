package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	sharedrequestcontext "github.com/StephenQiu30/hotkey-server/backend/internal/shared/requestcontext"
)

const (
	RawEvidenceRightsSubjectResponse = "raw_response"
	RawEvidenceRightsSubjectEndpoint = "source_endpoint"
	rawStoreFailureCode              = "OBJECT_STORE_FAILED"
	rawEvidenceObjectRootPrefix      = "source-raw/v1/"
	MaximumCaptureClockSkew          = 5 * time.Minute
)

// Clock makes archive-time validation deterministic and testable.
type Clock interface {
	Now() time.Time
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now().UTC() }

// EvidenceSelectorVerifier independently re-selects referenced bytes from an
// immutable snapshot. Unknown locator or selector versions must return an
// error; the archive use case never falls back to a caller-declared hash.
type EvidenceSelectorVerifier interface {
	Verify(EvidenceSelectorInputDTO) error
}

type StoreRawEvidenceCommand struct {
	SourceConnectionID      int64
	EvidenceKey             string
	ObjectKey               string
	Payload                 []byte
	PayloadSHA256           string
	CollectorProfileVersion string
	MIMEType                string
}

func (command StoreRawEvidenceCommand) Validate() error {
	if command.SourceConnectionID <= 0 || !validSHA256Hex(command.EvidenceKey) || !validSHA256Hex(command.PayloadSHA256) || command.Payload == nil {
		return fmt.Errorf("raw evidence store command identity is invalid")
	}
	profile, err := domain.NewCollectorProfileVersion(command.CollectorProfileVersion)
	if err != nil {
		return fmt.Errorf("raw evidence store command collector profile is invalid")
	}
	evidenceKey, err := domain.EvidenceSnapshotIdentity(command.PayloadSHA256, profile)
	if err != nil || evidenceKey != command.EvidenceKey || command.ObjectKey != RawEvidenceObjectKey(command.SourceConnectionID, command.EvidenceKey) {
		return fmt.Errorf("raw evidence store command identity is invalid")
	}
	digest := sha256.Sum256(command.Payload)
	if hex.EncodeToString(digest[:]) != command.PayloadSHA256 {
		return fmt.Errorf("raw evidence store command payload does not match SHA-256")
	}
	if command.MIMEType == "" || command.MIMEType != strings.TrimSpace(command.MIMEType) || len(command.MIMEType) > 255 || strings.ContainsAny(command.MIMEType, "\x00\r\n") {
		return fmt.Errorf("raw evidence store command MIME type is invalid")
	}
	return nil
}

type StoreRawEvidenceResult struct {
	SourceConnectionID      int64
	EvidenceKey             string
	ObjectKey               string
	PayloadSHA256           string
	CollectorProfileVersion string
	MIMEType                string
	SizeBytes               int64
}

func (result StoreRawEvidenceResult) ValidateAgainst(command StoreRawEvidenceCommand) error {
	if err := command.Validate(); err != nil {
		return err
	}
	if result.SourceConnectionID != command.SourceConnectionID || result.EvidenceKey != command.EvidenceKey ||
		result.ObjectKey != command.ObjectKey || result.PayloadSHA256 != command.PayloadSHA256 ||
		result.CollectorProfileVersion != command.CollectorProfileVersion || result.MIMEType != command.MIMEType ||
		result.SizeBytes != int64(len(command.Payload)) {
		return domain.ErrRawEvidenceConflict
	}
	return nil
}

type RawEvidenceStore interface {
	PutIfAbsent(context.Context, StoreRawEvidenceCommand) (StoreRawEvidenceResult, error)
}

// ReserveEvidenceSnapshotCommand contains the proposed first-capture facts.
// A repository must return the already-persisted facts unchanged when the
// endpoint-scoped EvidenceKey exists; recapture never updates CapturedAt,
// RetentionUntil, CollectionRunID, response metadata, or decision bindings.
type ReserveEvidenceSnapshotCommand struct {
	SourceConnectionID       int64
	CollectionRunID          int64
	StoreRawRightsDecisionID int64
	RetainRightsDecisionID   int64
	EvidenceKey              string
	ObjectKey                string
	PayloadSHA256            string
	CollectorProfileVersion  string
	MIMEType                 string
	SizeBytes                int64
	ResponseStatus           int
	RequestedURL             string
	FinalURL                 string
	RedirectChain            []string
	ResponseHeaders          RawResponseHeadersDTO
	CapturedAt               time.Time
	RetentionUntil           time.Time
}

type PersistedEvidenceSnapshotDTO struct {
	ID                       int64
	LifecycleState           string
	SourceConnectionID       int64
	CollectionRunID          int64
	StoreRawRightsDecisionID int64
	RetainRightsDecisionID   int64
	EvidenceKey              string
	ObjectKey                string
	PayloadSHA256            string
	CollectorProfileVersion  string
	MIMEType                 string
	SizeBytes                int64
	ResponseStatus           int
	RequestedURL             string
	FinalURL                 string
	RedirectChain            []string
	ResponseHeaders          RawResponseHeadersDTO
	CapturedAt               time.Time
	RetentionUntil           time.Time
}

type SourceObservationDTO struct {
	SourceConnectionID int64
	CollectionRunID    int64
	ExternalID         string
	UpstreamIdentity   string
	SourceCode         string
	ContentType        string
	Title              string
	Language           string
	Author             string
	SourceRecordURL    string
	CanonicalURL       string
	DiscussionURL      string
	BodyOrigin         string
	Completeness       string
	PublishedAt        *time.Time
	DiscoveredAt       time.Time
	CapturedAt         time.Time
	Evidence           RawEvidenceReferenceDTO
	Parties            []SourceObservationPartyDTO
}

type CommitEvidenceSnapshotCommand struct {
	SnapshotID                    int64
	StoreResult                   StoreRawEvidenceResult
	Observations                  []SourceObservationDTO
	TraceID                       string
	DocumentGenerationScheduledAt time.Time
}

type EvidenceSnapshotRepository interface {
	Reserve(context.Context, ReserveEvidenceSnapshotCommand) (PersistedEvidenceSnapshotDTO, error)
	Commit(context.Context, CommitEvidenceSnapshotCommand) (CommitEvidenceSnapshotResult, error)
	MarkFailed(context.Context, int64, string) error
}

// RawEvidenceRightsDecisionDTO is a single-action current authorization read
// by the archive use case.
type RawEvidenceRightsDecisionDTO struct {
	ID                      int64
	PolicyID                int64
	PolicyRevision          int64
	SourceConnectionID      int64
	SubjectType             string
	SubjectKey              string
	InputDigest             string
	Action                  string
	Decision                string
	EffectiveFrom           time.Time
	ExpiresAt               *time.Time
	RetentionDays           *int
	SupersedesDecisionID    *int64
	AuthorizedEvidenceKey   string
	AuthorizedPayloadSHA256 string
}

func (decision RawEvidenceRightsDecisionDTO) authorizes(action domain.RightsAction, sourceConnectionID int64, snapshot domain.EvidenceSnapshot, at time.Time) bool {
	decisionAction, decisionState, err := rawEvidenceRightsDecisionEntitiesFromDTO(decision)
	if err != nil {
		return false
	}
	exactSubject := decision.SubjectType == RawEvidenceRightsSubjectResponse && decision.SubjectKey == snapshot.Key &&
		decision.InputDigest == snapshot.PayloadSHA256
	endpointSubject := decision.SubjectType == RawEvidenceRightsSubjectEndpoint &&
		decision.SubjectKey == strconv.FormatInt(sourceConnectionID, 10) && validSHA256Hex(decision.InputDigest) &&
		decision.AuthorizedEvidenceKey == snapshot.Key && decision.AuthorizedPayloadSHA256 == snapshot.PayloadSHA256
	if decision.ID <= 0 || decision.SourceConnectionID != sourceConnectionID || (!exactSubject && !endpointSubject) ||
		decisionAction != action || decisionState != domain.RightsAllow || decision.PolicyID <= 0 || decision.PolicyRevision <= 0 ||
		decision.EffectiveFrom.IsZero() || at.IsZero() || at.Before(decision.EffectiveFrom) {
		return false
	}
	if action == domain.RightsActionRetain {
		if decision.RetentionDays == nil || *decision.RetentionDays <= 0 || *decision.RetentionDays > 3650 {
			return false
		}
	} else if decision.RetentionDays != nil {
		return false
	}
	if decision.ExpiresAt != nil && (!decision.ExpiresAt.After(decision.EffectiveFrom) || !at.Before(*decision.ExpiresAt)) {
		return false
	}
	if decision.SupersedesDecisionID != nil && (*decision.SupersedesDecisionID <= 0 || *decision.SupersedesDecisionID == decision.ID) {
		return false
	}
	return true
}

type ArchiveRawEvidenceCommand struct {
	SourceConnectionID int64
	CollectionRunID    int64
	Fetch              RawEvidenceFetchDTO
	StoreRawDecisions  map[string]RawEvidenceRightsDecisionDTO
	RetainDecisions    map[string]RawEvidenceRightsDecisionDTO
}

type ArchiveRawEvidenceResult struct {
	Snapshots []PersistedEvidenceSnapshotDTO
}

type rawArchiveAuthorization struct {
	storeRaw       RawEvidenceRightsDecisionDTO
	retain         RawEvidenceRightsDecisionDTO
	retentionUntil time.Time
}

type RawEvidenceArchiveServiceDependencies struct {
	Store               RawEvidenceStore
	Repository          EvidenceSnapshotRepository
	SelectorVerifier    EvidenceSelectorVerifier
	Clock               Clock
	MaxCaptureClockSkew time.Duration
}

type RawEvidenceArchiveService struct {
	store               RawEvidenceStore
	repository          EvidenceSnapshotRepository
	selectorVerifier    EvidenceSelectorVerifier
	clock               Clock
	maxCaptureClockSkew time.Duration
}

func NewRawEvidenceArchiveService(dependencies RawEvidenceArchiveServiceDependencies) (*RawEvidenceArchiveService, error) {
	if dependencies.Store == nil || dependencies.Repository == nil || dependencies.SelectorVerifier == nil {
		return nil, errors.New("raw evidence store, repository, and selector verifier are required")
	}
	if dependencies.Clock == nil {
		dependencies.Clock = wallClock{}
	}
	if dependencies.MaxCaptureClockSkew < 0 || dependencies.MaxCaptureClockSkew > MaximumCaptureClockSkew {
		return nil, fmt.Errorf("raw evidence capture clock skew must be between zero and %s", MaximumCaptureClockSkew)
	}
	return &RawEvidenceArchiveService{
		store: dependencies.Store, repository: dependencies.Repository, selectorVerifier: dependencies.SelectorVerifier,
		clock: dependencies.Clock, maxCaptureClockSkew: dependencies.MaxCaptureClockSkew,
	}, nil
}

func (service *RawEvidenceArchiveService) Archive(ctx context.Context, command ArchiveRawEvidenceCommand) (ArchiveRawEvidenceResult, error) {
	if service == nil || service.store == nil || service.repository == nil || service.selectorVerifier == nil || service.clock == nil {
		return ArchiveRawEvidenceResult{}, errors.New("raw evidence archive service is not initialized")
	}
	if command.SourceConnectionID <= 0 || command.CollectionRunID <= 0 {
		return ArchiveRawEvidenceResult{}, errors.New("raw evidence source and collection run are required")
	}
	now := service.clock.Now().UTC()
	if now.IsZero() {
		return ArchiveRawEvidenceResult{}, errors.New("raw evidence archive clock returned zero time")
	}

	fetchResult, err := rawEvidenceFetchEntityFromDTO(command.Fetch)
	if err != nil {
		return ArchiveRawEvidenceResult{}, fmt.Errorf("validate raw evidence fetch DTO: %w", err)
	}
	orderedSnapshots, snapshotsByKey, err := validateArchiveFetchResult(fetchResult, now, service.maxCaptureClockSkew)
	if err != nil {
		return ArchiveRawEvidenceResult{}, err
	}
	if len(orderedSnapshots) == 0 {
		return ArchiveRawEvidenceResult{Snapshots: []PersistedEvidenceSnapshotDTO{}}, nil
	}
	observations, err := archiveObservationDTOs(command.SourceConnectionID, command.CollectionRunID, fetchResult.Items, snapshotsByKey, service.selectorVerifier)
	if err != nil {
		return ArchiveRawEvidenceResult{}, err
	}
	authorizations := make(map[string]rawArchiveAuthorization, len(orderedSnapshots))
	for _, snapshot := range orderedSnapshots {
		storeRawDecision, found := command.StoreRawDecisions[snapshot.Key]
		if !found || !storeRawDecision.authorizes(domain.RightsActionStoreRaw, command.SourceConnectionID, snapshot, now) {
			return ArchiveRawEvidenceResult{}, fmt.Errorf("%w: explicit current store_raw allow is required", domain.ErrRawArchiveNotAuthorized)
		}
		retainDecision, found := command.RetainDecisions[snapshot.Key]
		if !found || !retainDecision.authorizes(domain.RightsActionRetain, command.SourceConnectionID, snapshot, now) {
			return ArchiveRawEvidenceResult{}, fmt.Errorf("%w: explicit current retain allow is required", domain.ErrRawArchiveNotAuthorized)
		}
		retentionUntil := snapshot.CapturedAt.Add(time.Duration(*retainDecision.RetentionDays) * 24 * time.Hour)
		if !retentionUntil.After(now) {
			return ArchiveRawEvidenceResult{}, fmt.Errorf("%w: raw evidence retention window has elapsed", domain.ErrRawArchiveNotAuthorized)
		}
		authorizations[snapshot.Key] = rawArchiveAuthorization{storeRaw: storeRawDecision, retain: retainDecision, retentionUntil: retentionUntil}
	}

	result := ArchiveRawEvidenceResult{Snapshots: make([]PersistedEvidenceSnapshotDTO, 0, len(orderedSnapshots))}
	for _, snapshot := range orderedSnapshots {
		authorization := authorizations[snapshot.Key]
		reservation := reserveEvidenceSnapshotCommandFromEntity(command.SourceConnectionID, command.CollectionRunID, snapshot, authorization)
		persisted, err := service.repository.Reserve(ctx, reservation)
		if err != nil {
			return ArchiveRawEvidenceResult{}, fmt.Errorf("reserve raw evidence snapshot: %w", err)
		}
		if err := validatePersistedEvidenceSnapshot(persisted, reservation, snapshot.Payload, now, service.maxCaptureClockSkew); err != nil {
			return ArchiveRawEvidenceResult{}, err
		}
		storeCommand := storeRawEvidenceCommandFromPersistence(persisted, snapshot.Payload)
		storeResult, err := service.store.PutIfAbsent(ctx, storeCommand)
		if err != nil {
			if persistedEvidenceLifecycleIs(persisted, domain.EvidenceLifecyclePending) {
				if markErr := service.repository.MarkFailed(ctx, persisted.ID, rawStoreFailureCode); markErr != nil {
					return ArchiveRawEvidenceResult{}, fmt.Errorf("store raw evidence: %w; mark snapshot failed: %v", err, markErr)
				}
			}
			return ArchiveRawEvidenceResult{}, fmt.Errorf("store raw evidence: %w", err)
		}
		if err := storeResult.ValidateAgainst(storeCommand); err != nil {
			if persistedEvidenceLifecycleIs(persisted, domain.EvidenceLifecyclePending) {
				_ = service.repository.MarkFailed(ctx, persisted.ID, rawStoreFailureCode)
			}
			return ArchiveRawEvidenceResult{}, fmt.Errorf("verify raw evidence store result: %w", err)
		}
		commitResult, err := service.repository.Commit(ctx, CommitEvidenceSnapshotCommand{
			SnapshotID: persisted.ID, StoreResult: storeResult, Observations: observations[snapshot.Key],
			TraceID: sharedrequestcontext.TraceID(ctx), DocumentGenerationScheduledAt: now,
		})
		if err != nil {
			return ArchiveRawEvidenceResult{}, fmt.Errorf("commit raw evidence observations: %w", err)
		}
		committed := commitResult.Snapshot
		if committed.ID != persisted.ID || !persistedEvidenceLifecycleIs(committed, domain.EvidenceLifecycleAvailable) || !samePersistedEvidenceFacts(persisted, committed) {
			return ArchiveRawEvidenceResult{}, domain.ErrRawEvidenceConflict
		}
		if err := validateCommittedEvidenceReferences(commitResult.EvidenceReferences, persisted.ID, len(observations[snapshot.Key])); err != nil {
			return ArchiveRawEvidenceResult{}, err
		}
		result.Snapshots = append(result.Snapshots, committed)
	}
	return result, nil
}

func validateCommittedEvidenceReferences(references []CommittedEvidenceReferenceDTO, snapshotID int64, expected int) error {
	if references == nil || len(references) != expected {
		return domain.ErrRawEvidenceConflict
	}
	seen := make(map[int64]struct{}, len(references))
	for _, reference := range references {
		if err := reference.Validate(); err != nil || reference.EvidenceSnapshotID != snapshotID {
			return domain.ErrRawEvidenceConflict
		}
		if _, duplicate := seen[reference.EvidenceReferenceID]; duplicate {
			return domain.ErrRawEvidenceConflict
		}
		seen[reference.EvidenceReferenceID] = struct{}{}
	}
	return nil
}

func RawEvidenceObjectKey(sourceConnectionID int64, evidenceKey string) string {
	if sourceConnectionID <= 0 || !validSHA256Hex(evidenceKey) {
		return ""
	}
	return fmt.Sprintf("%s%d/%s/%s.raw", rawEvidenceObjectRootPrefix, sourceConnectionID, evidenceKey[:2], evidenceKey)
}

func validateArchiveFetchResult(result domain.FetchResult, now time.Time, maxClockSkew time.Duration) ([]domain.EvidenceSnapshot, map[string]domain.EvidenceSnapshot, error) {
	ordered := make([]domain.EvidenceSnapshot, 0, len(result.Snapshots))
	byKey := make(map[string]domain.EvidenceSnapshot, len(result.Snapshots))
	for _, snapshot := range result.Snapshots {
		rebuilt, err := domain.NewEvidenceSnapshot(snapshot)
		if err != nil {
			return nil, nil, fmt.Errorf("validate evidence snapshot: %w", err)
		}
		if snapshot.CapturedAt.After(now.Add(maxClockSkew)) {
			return nil, nil, fmt.Errorf("evidence snapshot capture time exceeds allowed clock skew")
		}
		if existing, found := byKey[rebuilt.Key]; found {
			if existing.PayloadSHA256 != rebuilt.PayloadSHA256 || existing.CollectorProfileVersion != rebuilt.CollectorProfileVersion || string(existing.Payload) != string(rebuilt.Payload) {
				return nil, nil, domain.ErrRawEvidenceConflict
			}
			continue
		}
		ordered = append(ordered, rebuilt)
		byKey[rebuilt.Key] = rebuilt
	}
	return ordered, byKey, nil
}

func archiveObservationDTOs(sourceConnectionID, collectionRunID int64, items []domain.SourceItem, snapshots map[string]domain.EvidenceSnapshot, verifier EvidenceSelectorVerifier) (map[string][]SourceObservationDTO, error) {
	grouped := make(map[string][]SourceObservationDTO, len(snapshots))
	for _, item := range items {
		normalized, err := domain.NormalizeSourceItem(item)
		if err != nil {
			return nil, fmt.Errorf("normalize raw evidence source item: %w", err)
		}
		if len(normalized.EvidenceReferences) == 0 {
			return nil, fmt.Errorf("raw evidence source item requires a verifiable evidence reference")
		}
		upstreamIdentity := sourceObservationIdentity(normalized)
		for _, reference := range normalized.EvidenceReferences {
			snapshot, found := snapshots[reference.SnapshotKey]
			if !found {
				return nil, fmt.Errorf("raw evidence source item references an unknown snapshot")
			}
			if normalized.ObservedAt.After(snapshot.CapturedAt) {
				return nil, fmt.Errorf("raw evidence observation time exceeds response capture time")
			}
			if err := verifier.Verify(evidenceSelectorInputDTOFromEntities(snapshot, reference)); err != nil {
				return nil, fmt.Errorf("%w: %v", domain.ErrEvidenceSelection, err)
			}
			observation, err := sourceObservationDTOFromEntity(sourceConnectionID, collectionRunID, normalized, upstreamIdentity, snapshot, reference)
			if err != nil {
				return nil, err
			}
			grouped[reference.SnapshotKey] = append(grouped[reference.SnapshotKey], observation)
		}
	}
	return grouped, nil
}

func reserveEvidenceSnapshotCommandFromEntity(sourceConnectionID, collectionRunID int64, snapshot domain.EvidenceSnapshot, authorization rawArchiveAuthorization) ReserveEvidenceSnapshotCommand {
	return ReserveEvidenceSnapshotCommand{
		SourceConnectionID: sourceConnectionID, CollectionRunID: collectionRunID,
		StoreRawRightsDecisionID: authorization.storeRaw.ID, RetainRightsDecisionID: authorization.retain.ID,
		EvidenceKey: snapshot.Key, ObjectKey: RawEvidenceObjectKey(sourceConnectionID, snapshot.Key),
		PayloadSHA256: snapshot.PayloadSHA256, CollectorProfileVersion: snapshot.CollectorProfileVersion.String(),
		MIMEType: snapshot.MIMEType, SizeBytes: int64(len(snapshot.Payload)), ResponseStatus: snapshot.StatusCode,
		RequestedURL: snapshot.RequestedURL, FinalURL: snapshot.FinalURL,
		RedirectChain: append([]string(nil), snapshot.RedirectChain...), ResponseHeaders: rawResponseHeadersDTOFromEntity(snapshot.ResponseHeaders),
		CapturedAt: snapshot.CapturedAt, RetentionUntil: authorization.retentionUntil,
	}
}

func storeRawEvidenceCommandFromPersistence(persisted PersistedEvidenceSnapshotDTO, payload []byte) StoreRawEvidenceCommand {
	return StoreRawEvidenceCommand{
		SourceConnectionID: persisted.SourceConnectionID, EvidenceKey: persisted.EvidenceKey, ObjectKey: persisted.ObjectKey,
		Payload: append([]byte(nil), payload...), PayloadSHA256: persisted.PayloadSHA256,
		CollectorProfileVersion: persisted.CollectorProfileVersion, MIMEType: persisted.MIMEType,
	}
}

func validatePersistedEvidenceSnapshot(persisted PersistedEvidenceSnapshotDTO, reservation ReserveEvidenceSnapshotCommand, payload []byte, now time.Time, maxClockSkew time.Duration) error {
	lifecycleState, err := evidenceLifecycleEntityFromString(persisted.LifecycleState)
	if err != nil || (lifecycleState != domain.EvidenceLifecyclePending && lifecycleState != domain.EvidenceLifecycleAvailable) ||
		persisted.ID <= 0 ||
		persisted.SourceConnectionID != reservation.SourceConnectionID || persisted.EvidenceKey != reservation.EvidenceKey ||
		persisted.ObjectKey != reservation.ObjectKey || persisted.PayloadSHA256 != reservation.PayloadSHA256 ||
		persisted.CollectorProfileVersion != reservation.CollectorProfileVersion || persisted.SizeBytes != reservation.SizeBytes ||
		persisted.CollectionRunID <= 0 || persisted.StoreRawRightsDecisionID <= 0 || persisted.RetainRightsDecisionID <= 0 ||
		persisted.CapturedAt.After(now.Add(maxClockSkew)) || !persisted.RetentionUntil.After(persisted.CapturedAt) || !persisted.RetentionUntil.After(now) {
		return domain.ErrRawEvidenceConflict
	}
	profile, err := domain.NewCollectorProfileVersion(persisted.CollectorProfileVersion)
	if err != nil {
		return domain.ErrRawEvidenceConflict
	}
	responseHeaders, err := rawResponseHeadersEntityFromDTO(persisted.ResponseHeaders)
	if err != nil {
		return domain.ErrRawEvidenceConflict
	}
	rebuilt, err := domain.NewEvidenceSnapshot(domain.EvidenceSnapshot{
		Key: persisted.EvidenceKey, Payload: payload, PayloadSHA256: persisted.PayloadSHA256, CollectorProfileVersion: profile,
		MIMEType: persisted.MIMEType, StatusCode: persisted.ResponseStatus,
		RequestedURL: persisted.RequestedURL, FinalURL: persisted.FinalURL,
		RedirectChain: persisted.RedirectChain, ResponseHeaders: responseHeaders, CapturedAt: persisted.CapturedAt,
	})
	if err != nil || rebuilt.Key != persisted.EvidenceKey {
		return domain.ErrRawEvidenceConflict
	}
	return nil
}

func samePersistedEvidenceFacts(left, right PersistedEvidenceSnapshotDTO) bool {
	return left.SourceConnectionID == right.SourceConnectionID && left.CollectionRunID == right.CollectionRunID &&
		left.StoreRawRightsDecisionID == right.StoreRawRightsDecisionID && left.RetainRightsDecisionID == right.RetainRightsDecisionID &&
		left.EvidenceKey == right.EvidenceKey && left.ObjectKey == right.ObjectKey && left.PayloadSHA256 == right.PayloadSHA256 &&
		left.CollectorProfileVersion == right.CollectorProfileVersion && left.MIMEType == right.MIMEType && left.SizeBytes == right.SizeBytes &&
		left.ResponseStatus == right.ResponseStatus && left.RequestedURL == right.RequestedURL && left.FinalURL == right.FinalURL &&
		strings.Join(left.RedirectChain, "\x00") == strings.Join(right.RedirectChain, "\x00") && left.ResponseHeaders.Equal(right.ResponseHeaders) &&
		left.CapturedAt.Equal(right.CapturedAt) && left.RetentionUntil.Equal(right.RetentionUntil)
}

func sourceObservationDTOFromEntity(sourceConnectionID, collectionRunID int64, item domain.SourceItem, upstreamIdentity string, snapshot domain.EvidenceSnapshot, reference domain.EvidenceReference) (SourceObservationDTO, error) {
	if len(item.Author) > 512 || strings.ContainsAny(item.Author, "\x00\r\n") {
		return SourceObservationDTO{}, fmt.Errorf("raw evidence observation author is invalid")
	}
	language := item.Language
	if language == "" {
		language = "und"
	}
	bodyOrigin, completeness := rawBodySemantics(item)
	canonicalURL := item.URL
	if canonicalURL != "" {
		parsed, err := url.Parse(canonicalURL)
		if err != nil || parsed == nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.Fragment != "" || len(canonicalURL) > 2048 {
			return SourceObservationDTO{}, fmt.Errorf("raw evidence observation URL is invalid")
		}
	}
	var publishedAt *time.Time
	if item.PublishedAt != nil {
		value := item.PublishedAt.UTC()
		publishedAt = &value
	}
	return SourceObservationDTO{
		SourceConnectionID: sourceConnectionID, CollectionRunID: collectionRunID, ExternalID: item.ExternalID, UpstreamIdentity: upstreamIdentity,
		SourceCode: item.SourceCode, ContentType: item.ContentType, Title: item.Title, Language: language, Author: item.Author,
		SourceRecordURL: snapshot.FinalURL, CanonicalURL: canonicalURL, DiscussionURL: item.DiscussionURL, BodyOrigin: bodyOrigin, Completeness: completeness,
		PublishedAt: publishedAt, DiscoveredAt: item.ObservedAt.UTC(), CapturedAt: snapshot.CapturedAt.UTC(),
		Evidence: rawEvidenceReferenceDTOFromEntity(reference), Parties: sourceObservationPartyDTOsFromEntities(item.Parties),
	}, nil
}

func persistedEvidenceLifecycleIs(snapshot PersistedEvidenceSnapshotDTO, expected domain.EvidenceLifecycleState) bool {
	state, err := evidenceLifecycleEntityFromString(snapshot.LifecycleState)
	return err == nil && state == expected
}

func sourceObservationIdentity(item domain.SourceItem) string {
	digest := sha256.New()
	// Body is transient raw input. Exact selected-payload digests live on the
	// evidence locator; observation identity must not smuggle Body-derived
	// material into a repository command.
	for _, value := range []string{item.SourceCode, item.ExternalID, item.ContentType, item.Title, item.Language, item.URL, item.DiscussionURL, item.Author} {
		_, _ = digest.Write([]byte(fmt.Sprintf("%d:%s", len(value), value)))
	}
	for _, party := range item.Parties {
		for _, value := range []string{string(party.Role), string(party.Kind), party.IdentityNamespace, party.ExternalID, party.DisplayName, party.HomepageURL} {
			_, _ = digest.Write([]byte(fmt.Sprintf("%d:%s", len(value), value)))
		}
	}
	if item.PublishedAt != nil {
		_, _ = digest.Write([]byte(item.PublishedAt.UTC().Format(time.RFC3339Nano)))
	}
	return fmt.Sprintf("%x", digest.Sum(nil))
}

func rawBodySemantics(item domain.SourceItem) (string, string) {
	completeness := "unknown"
	switch item.EvidenceCompleteness {
	case domain.EvidenceCompletenessFullBody:
		completeness = "full"
	case domain.EvidenceCompletenessSummaryOnly:
		completeness = "summary"
	case domain.EvidenceCompletenessMetadataOnly:
		completeness = "metadata_only"
	}
	switch item.SourceCode {
	case "rss":
		if completeness == "summary" || completeness == "metadata_only" {
			return "feed_summary", completeness
		}
		return "feed_content", completeness
	case "hacker_news", "x", "bilibili", "weibo":
		return "platform_post", completeness
	case "bing_grounding", "google_agent_search":
		if completeness == "summary" {
			completeness = "snippet"
		}
		return "search_snippet", completeness
	default:
		return "api_content", completeness
	}
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func validSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
