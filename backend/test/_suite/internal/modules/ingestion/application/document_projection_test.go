package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type documentProjectionPublisherFake struct {
	commands []knowledgeapplication.PublishProjectionCommand
	result   knowledgeapplication.PublishProjectionResult
	err      error
}

func (publisher *documentProjectionPublisherFake) Publish(_ context.Context, command knowledgeapplication.PublishProjectionCommand) (knowledgeapplication.PublishProjectionResult, error) {
	copy := command
	copy.Content = append([]byte(nil), command.Content...)
	publisher.commands = append(publisher.commands, copy)
	return publisher.result, publisher.err
}

type derivedArtifactRepositoryFake struct {
	reserveResult ReserveDerivedArtifactResult
	commitResult  CommitDerivedArtifactResult
	reserveErr    error
	commitErr     error
	markFailedErr error
	quarantineErr error
	reserves      []ReserveDerivedArtifactCommand
	commits       []CommitDerivedArtifactCommand
	failed        []MarkDerivedArtifactFailedCommand
	quarantined   []QuarantineDerivedArtifactCommand
}

func (repository *derivedArtifactRepositoryFake) Reserve(_ context.Context, command ReserveDerivedArtifactCommand) (ReserveDerivedArtifactResult, error) {
	repository.reserves = append(repository.reserves, command)
	return repository.reserveResult, repository.reserveErr
}

func (repository *derivedArtifactRepositoryFake) Commit(_ context.Context, command CommitDerivedArtifactCommand) (CommitDerivedArtifactResult, error) {
	repository.commits = append(repository.commits, command)
	return repository.commitResult, repository.commitErr
}

func (repository *derivedArtifactRepositoryFake) MarkFailed(_ context.Context, command MarkDerivedArtifactFailedCommand) (DerivedArtifactDTO, error) {
	repository.failed = append(repository.failed, command)
	artifact := repository.reserveResult.Artifact
	artifact.LifecycleState = DerivedArtifactFailed
	artifact.FailureCode = command.FailureCode
	artifact.Active = false
	return artifact, repository.markFailedErr
}

func (repository *derivedArtifactRepositoryFake) Quarantine(_ context.Context, command QuarantineDerivedArtifactCommand) (DerivedArtifactDTO, error) {
	repository.quarantined = append(repository.quarantined, command)
	artifact := repository.reserveResult.Artifact
	artifact.LifecycleState = DerivedArtifactQuarantined
	artifact.Active = false
	return artifact, repository.quarantineErr
}

type documentVersionLifecycleFake struct {
	current     DocumentVersionDTO
	transitions []TransitionDocumentVersionCommand
	err         error
}

func (lifecycle *documentVersionLifecycleFake) TransitionDocumentVersion(_ context.Context, command TransitionDocumentVersionCommand) (TransitionDocumentVersionResult, error) {
	lifecycle.transitions = append(lifecycle.transitions, command)
	if lifecycle.err != nil {
		return TransitionDocumentVersionResult{}, lifecycle.err
	}
	if lifecycle.current.ID != command.DocumentVersionID || lifecycle.current.Version != command.ExpectedVersion {
		return TransitionDocumentVersionResult{}, sharedrepository.ErrConflict
	}
	lifecycle.current.Version++
	lifecycle.current.LifecycleState = command.To
	lifecycle.current.DisplayPrivateRightsDecisionID = cloneOptionalInt64(command.DisplayPrivateRightsDecisionID)
	return TransitionDocumentVersionResult{DocumentVersion: lifecycle.current}, nil
}

func TestDocumentProjectionServicePublishesCommitsAndAdvancesReadableLifecycle(t *testing.T) {
	t.Parallel()
	fixture := newDocumentProjectionFixture(t, DocumentPolicyPending, 1)
	displayDecisionID := int64(73)
	command := fixture.command
	command.DisplayPrivateRightsDecisionID = &displayDecisionID

	result, err := fixture.service.Project(context.Background(), command)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if len(fixture.repository.reserves) != 1 || len(fixture.publisher.commands) != 1 || len(fixture.repository.commits) != 1 {
		t.Fatalf("reserve/publish/commit calls = %d/%d/%d", len(fixture.repository.reserves), len(fixture.publisher.commands), len(fixture.repository.commits))
	}
	reservation := fixture.repository.reserves[0]
	digest := fmt.Sprintf("%x", sha256.Sum256(command.ProjectionBytes))
	if reservation.DocumentVersionID != command.DocumentVersionID || reservation.ExpectedDocumentVersion != command.ExpectedDocumentVersion ||
		reservation.ArtifactType != DerivedArtifactMarkdown || reservation.TransformerProfileSHA256 != command.TransformerProfileSHA256 ||
		reservation.SHA256 != digest || reservation.SizeBytes != int64(len(command.ProjectionBytes)) ||
		reservation.MIMEType != "text/markdown; charset=utf-8" || reservation.StoreDerivedRightsDecisionID != 41 || reservation.RetainRightsDecisionID != 42 {
		t.Fatalf("Reserve() command = %#v", reservation)
	}
	if reservation.AnchorMap == nil || reservation.AnchorMap.PlaintextSHA256 != fixture.command.AnchorMap.PlaintextSHA256 ||
		reservation.AnchorMap.MarkdownSHA256 != digest || reservation.AnchorMap.AnchorMapSHA256 != fixture.command.AnchorMap.AnchorMapSHA256 ||
		len(fixture.repository.commits[0].AnchorBlocks) != 1 {
		t.Fatalf("anchor map reserve/commit = %#v/%#v", reservation.AnchorMap, fixture.repository.commits[0].AnchorBlocks)
	}
	if _, exposed := reflect.TypeOf(reservation).FieldByName("ProjectionBytes"); exposed {
		t.Fatal("ReserveDerivedArtifactCommand exposes projection bytes")
	}
	publish := fixture.publisher.commands[0]
	if publish.DocumentID != 7 || publish.DocumentVersionID != 19 || publish.Format != knowledgeapplication.ProjectionFormatMarkdown ||
		publish.TransformerProfileSHA256 != command.TransformerProfileSHA256 || publish.SHA256 != digest || string(publish.Content) != string(command.ProjectionBytes) {
		t.Fatalf("Publish() command = %#v", publish)
	}
	if commit := fixture.repository.commits[0]; commit.ArtifactID != 29 || commit.Receipt.VaultRelativePath != fixture.publisher.result.RelativePath || commit.Receipt.SHA256 != digest {
		t.Fatalf("Commit() command = %#v", commit)
	}
	wantStates := []string{
		DocumentDerivedPending, DocumentDerivedAvailable, DocumentReadable,
	}
	if len(fixture.lifecycle.transitions) != len(wantStates) {
		t.Fatalf("lifecycle transitions = %#v", fixture.lifecycle.transitions)
	}
	for index, state := range wantStates {
		if fixture.lifecycle.transitions[index].To != state {
			t.Fatalf("lifecycle transition %d = %s, want %s", index, fixture.lifecycle.transitions[index].To, state)
		}
	}
	if result.Artifact.ID != 29 || !result.Artifact.Active || result.Artifact.LifecycleState != DerivedArtifactAvailable ||
		result.DocumentVersion.Version != 4 || result.DocumentVersion.LifecycleState != DocumentReadable {
		t.Fatalf("Project() result = %#v", result)
	}
	if _, exposed := reflect.TypeOf(result.Artifact).FieldByName("ProjectionBytes"); exposed {
		t.Fatal("ProjectDocumentResult exposes projection bytes")
	}
}

func TestDocumentProjectionServiceMarksOrdinaryFailuresWithoutQuarantine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		publisher   error
		commit      error
		failureCode string
	}{
		{name: "vault publish", publisher: knowledgeapplication.ErrProjectionUnavailable, failureCode: "vault_publish_failed"},
		{name: "database commit", commit: sharedrepository.ErrUnavailable, failureCode: "artifact_commit_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newDocumentProjectionFixture(t, DocumentPolicyPending, 1)
			fixture.publisher.err = test.publisher
			fixture.repository.commitErr = test.commit
			_, err := fixture.service.Project(context.Background(), fixture.command)
			if err == nil {
				t.Fatal("Project() accepted a failed projection")
			}
			if len(fixture.repository.failed) != 1 || fixture.repository.failed[0].FailureCode != test.failureCode {
				t.Fatalf("failed artifact commands = %#v", fixture.repository.failed)
			}
			if len(fixture.repository.quarantined) != 0 {
				t.Fatalf("ordinary failure quarantined artifact: %#v", fixture.repository.quarantined)
			}
			if got := fixture.lifecycle.current.LifecycleState; got != DocumentDerivedFailed {
				t.Fatalf("document lifecycle = %s, want derive_failed", got)
			}
		})
	}
}

func TestDocumentProjectionServiceQuarantinesOnlyProvenContentConflict(t *testing.T) {
	t.Parallel()
	fixture := newDocumentProjectionFixture(t, DocumentDerivedPending, 2)
	fixture.publisher.err = knowledgeapplication.ErrProjectionConflict

	_, err := fixture.service.Project(context.Background(), fixture.command)
	if !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Project() error = %v, want conflict", err)
	}
	if len(fixture.repository.quarantined) != 1 || len(fixture.repository.failed) != 0 {
		t.Fatalf("quarantine/failed commands = %#v/%#v", fixture.repository.quarantined, fixture.repository.failed)
	}
	if fixture.lifecycle.current.LifecycleState != DocumentQuarantined {
		t.Fatalf("document lifecycle = %s, want quarantined", fixture.lifecycle.current.LifecycleState)
	}
}

func TestDocumentProjectionServiceUsesReserveConflictEvidenceBeforeQuarantine(t *testing.T) {
	t.Parallel()
	withEvidence := newDocumentProjectionFixture(t, DocumentDerivedPending, 2)
	withEvidence.repository.reserveErr = fmt.Errorf("%w: %w", sharedrepository.ErrConflict, ErrDerivedArtifactContentConflict)
	if _, err := withEvidence.service.Project(context.Background(), withEvidence.command); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Project(reserve content conflict) error = %v, want conflict", err)
	}
	if len(withEvidence.repository.quarantined) != 1 || withEvidence.lifecycle.current.LifecycleState != DocumentQuarantined {
		t.Fatalf("evidenced reserve conflict was not quarantined: %#v/%s", withEvidence.repository.quarantined, withEvidence.lifecycle.current.LifecycleState)
	}

	withoutEvidence := newDocumentProjectionFixture(t, DocumentDerivedPending, 2)
	withoutEvidence.repository.reserveErr = sharedrepository.ErrConflict
	if _, err := withoutEvidence.service.Project(context.Background(), withoutEvidence.command); !errors.Is(err, sharedrepository.ErrConflict) {
		t.Fatalf("Project(rights conflict) error = %v, want conflict", err)
	}
	if len(withoutEvidence.repository.quarantined) != 0 || len(withoutEvidence.lifecycle.transitions) != 0 {
		t.Fatalf("unevidenced reserve conflict caused quarantine: %#v/%#v", withoutEvidence.repository.quarantined, withoutEvidence.lifecycle.transitions)
	}
}

func TestDocumentProjectionPersistenceDTOsNeverContainProjectionBytes(t *testing.T) {
	t.Parallel()
	values := []any{
		ReserveDerivedArtifactCommand{}, ReserveDerivedArtifactResult{}, ProjectionReceiptDTO{},
		CommitDerivedArtifactCommand{}, CommitDerivedArtifactResult{}, MarkDerivedArtifactFailedCommand{},
		QuarantineDerivedArtifactCommand{}, ProjectDocumentResult{},
	}
	for _, value := range values {
		if _, exposed := reflect.TypeOf(value).FieldByName("ProjectionBytes"); exposed {
			t.Fatalf("%T exposes projection bytes outside ProjectDocumentCommand", value)
		}
	}
}

func TestDocumentProjectionServiceIsIdempotentAfterLifecycleAdvanced(t *testing.T) {
	t.Parallel()
	fixture := newDocumentProjectionFixture(t, DocumentReadable, 4)
	fixture.repository.reserveResult.Artifact.LifecycleState = DerivedArtifactAvailable
	fixture.repository.reserveResult.Artifact.Active = true
	availableAt := fixture.repository.reserveResult.Artifact.UpdatedAt
	fixture.repository.reserveResult.Artifact.AvailableAt = &availableAt
	fixture.repository.commitResult.Artifact = fixture.repository.reserveResult.Artifact
	fixture.repository.commitResult.DocumentVersion = fixture.lifecycle.current
	fixture.command.ExpectedDocumentVersion = 1

	result, err := fixture.service.Project(context.Background(), fixture.command)
	if err != nil || result.DocumentVersion.Version != 4 || result.DocumentVersion.LifecycleState != DocumentReadable {
		t.Fatalf("idempotent Project() = %#v/%v", result, err)
	}
	if len(fixture.lifecycle.transitions) != 0 {
		t.Fatalf("idempotent Project() changed lifecycle: %#v", fixture.lifecycle.transitions)
	}
}

func TestDocumentProjectionServiceDoesNotReactivateSupersededIdempotentArtifact(t *testing.T) {
	t.Parallel()
	fixture := newDocumentProjectionFixture(t, DocumentReadable, 4)
	fixture.repository.reserveResult.Artifact.LifecycleState = DerivedArtifactAvailable
	availableAt := fixture.repository.reserveResult.Artifact.UpdatedAt
	fixture.repository.reserveResult.Artifact.AvailableAt = &availableAt
	fixture.repository.reserveResult.Artifact.Active = false
	fixture.repository.commitResult.Artifact = fixture.repository.reserveResult.Artifact
	fixture.repository.commitResult.DocumentVersion = fixture.lifecycle.current
	fixture.command.ExpectedDocumentVersion = 1

	result, err := fixture.service.Project(context.Background(), fixture.command)
	if err != nil || result.Artifact.Active || result.DocumentVersion.LifecycleState != DocumentReadable {
		t.Fatalf("Project(superseded retry) = %#v/%v", result, err)
	}
	if len(fixture.repository.quarantined) != 0 || len(fixture.lifecycle.transitions) != 0 {
		t.Fatalf("superseded retry changed state: %#v/%#v", fixture.repository.quarantined, fixture.lifecycle.transitions)
	}
}

func TestDocumentProjectionServiceRejectsNonCanonicalBytesBeforePorts(t *testing.T) {
	t.Parallel()
	fixture := newDocumentProjectionFixture(t, DocumentPolicyPending, 1)
	for name, content := range map[string][]byte{
		"invalid UTF-8": {0xff},
		"non NFC":       []byte("Cafe\u0301"),
		"CR newline":    []byte("line one\r\nline two"),
	} {
		t.Run(name, func(t *testing.T) {
			command := fixture.command
			command.ProjectionBytes = content
			if _, err := fixture.service.Project(context.Background(), command); !errors.Is(err, sharedrepository.ErrInvalidInput) {
				t.Fatalf("Project() error = %v, want invalid input", err)
			}
		})
	}
	if len(fixture.repository.reserves) != 0 || len(fixture.publisher.commands) != 0 {
		t.Fatalf("invalid bytes reached ports: %#v/%#v", fixture.repository.reserves, fixture.publisher.commands)
	}
}

func TestDocumentProjectionServiceRequiresAnchorMapOnlyForMarkdown(t *testing.T) {
	t.Parallel()
	markdown := newDocumentProjectionFixture(t, DocumentPolicyPending, 1)
	validMap := cloneProjectAnchorMap(markdown.command.AnchorMap)
	markdown.command.AnchorMap = nil
	if _, err := markdown.service.Project(context.Background(), markdown.command); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("Project(markdown without map) error = %v, want invalid input", err)
	}

	plaintext := newDocumentProjectionFixture(t, DocumentPolicyPending, 1)
	plaintext.command.ArtifactType = DocumentProjectionPlaintext
	plaintext.command.AnchorMap = validMap
	if _, err := plaintext.service.Project(context.Background(), plaintext.command); !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("Project(plaintext with map) error = %v, want invalid input", err)
	}
}

type documentProjectionFixture struct {
	service    *DocumentProjectionService
	publisher  *documentProjectionPublisherFake
	repository *derivedArtifactRepositoryFake
	lifecycle  *documentVersionLifecycleFake
	command    ProjectDocumentCommand
}

func newDocumentProjectionFixture(t *testing.T, state string, version int64) documentProjectionFixture {
	t.Helper()
	profile := strings.Repeat("a", 64)
	content := []byte("# Archived\n\n正文\n")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	createdAt := time.Date(2026, time.August, 9, 9, 0, 0, 0, time.UTC)
	documentVersion := DocumentVersionDTO{
		ID: 19, Version: version, DocumentID: 7, SourceObservationID: 13, RevisionNo: 1,
		VersionKey: strings.Repeat("c", 64), BodyOrigin: BodyOriginFeedContent,
		Completeness: BodyCompletenessFull, WordCount: 2, Language: "zh",
		ContentSHA256: strings.Repeat("b", 64), ExtractorVersion: "feed-v1",
		ExtractorProfileVersion: "profile-v1", ExtractorProfileSHA256: strings.Repeat("d", 64),
		LifecycleState: state, CapturedAt: createdAt, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	artifact := DerivedArtifactDTO{
		ID: 29, SourceConnectionID: 3, DocumentVersionID: 19,
		StoreDerivedRightsDecisionID: 41, RetainRightsDecisionID: 42,
		ArtifactType: DerivedArtifactMarkdown, TransformerProfileSHA256: profile,
		MIMEType: "text/markdown; charset=utf-8", SHA256: digest, SizeBytes: int64(len(content)),
		LifecycleState: DerivedArtifactPending,
		RetentionUntil: time.Date(2026, time.September, 8, 9, 0, 0, 0, time.UTC),
		CreatedAt:      createdAt, UpdatedAt: createdAt,
	}
	anchorMap := projectDocumentAnchorMapForTest("Archived\n\n正文", string(content))
	artifact.AnchorMap = &DerivedArtifactAnchorMapDTO{
		NormalizationVersion: anchorMap.NormalizationVersion, AnchorMapProfileVersion: anchorMap.AnchorMapProfileVersion,
		PlaintextSHA256: anchorMap.PlaintextSHA256, MarkdownSHA256: anchorMap.MarkdownSHA256, AnchorMapSHA256: anchorMap.AnchorMapSHA256,
	}
	relativePath := "documents/7/19/markdown/" + profile + ".md"
	publisher := &documentProjectionPublisherFake{result: knowledgeapplication.PublishProjectionResult{
		DocumentID: 7, DocumentVersionID: 19, Format: knowledgeapplication.ProjectionFormatMarkdown,
		TransformerProfileSHA256: profile, RelativePath: relativePath,
		MIMEType: artifact.MIMEType, SHA256: digest, SizeBytes: artifact.SizeBytes,
	}}
	repository := &derivedArtifactRepositoryFake{
		reserveResult: ReserveDerivedArtifactResult{
			Artifact: artifact, DocumentID: 7, VaultRelativePath: relativePath, DocumentVersion: documentVersion,
		},
		commitResult: CommitDerivedArtifactResult{
			Artifact: func() DerivedArtifactDTO {
				available := artifact
				available.LifecycleState = DerivedArtifactAvailable
				available.Active = true
				availableAt := available.UpdatedAt
				available.AvailableAt = &availableAt
				return available
			}(),
			DocumentID: 7, DocumentVersion: documentVersion,
		},
	}
	lifecycle := &documentVersionLifecycleFake{current: documentVersion}
	service, err := NewDocumentProjectionService(DocumentProjectionDependencies{
		Publisher: publisher, Repository: repository, DocumentVersions: lifecycle,
	})
	if err != nil {
		t.Fatalf("NewDocumentProjectionService() error = %v", err)
	}
	return documentProjectionFixture{
		service: service, publisher: publisher, repository: repository, lifecycle: lifecycle,
		command: ProjectDocumentCommand{
			DocumentVersionID: 19, ExpectedDocumentVersion: version,
			ArtifactType: DocumentProjectionMarkdown, TransformerProfileSHA256: profile,
			StoreDerivedRightsDecisionID: 41, RetainRightsDecisionID: 42,
			ProjectionBytes: append([]byte(nil), content...),
			AnchorMap:       anchorMap,
		},
	}
}

func projectDocumentAnchorMapForTest(plaintext, markdown string) *ProjectDocumentAnchorMapCommand {
	result := MapDocumentTextResult{
		Plaintext: plaintext, NormalizationVersion: CanonicalDocumentTextNormalizationVersion,
		AnchorMapProfileVersion: CanonicalDocumentAnchorMapProfileVersion,
		PlaintextSHA256:         documentAnchorTestSHA(plaintext), MarkdownSHA256: documentAnchorTestSHA(markdown),
		Blocks: []DocumentAnchorBlockDTO{{
			Ordinal: 0, PlaintextUTF8ByteStart: 0, PlaintextUTF8ByteEnd: int64(len(plaintext)),
			MarkdownUTF8ByteStart: 0, MarkdownUTF8ByteEnd: int64(len(markdown)),
			MarkdownAnchor: DocumentMarkdownAnchor(0, plaintext),
		}},
	}
	result.AnchorMapSHA256 = DocumentAnchorMapSHA256(result)
	return &ProjectDocumentAnchorMapCommand{
		Plaintext: result.Plaintext, NormalizationVersion: result.NormalizationVersion,
		AnchorMapProfileVersion: result.AnchorMapProfileVersion, PlaintextSHA256: result.PlaintextSHA256,
		MarkdownSHA256: result.MarkdownSHA256, AnchorMapSHA256: result.AnchorMapSHA256,
		Blocks: append([]DocumentAnchorBlockDTO(nil), result.Blocks...),
	}
}

func cloneProjectAnchorMap(value *ProjectDocumentAnchorMapCommand) *ProjectDocumentAnchorMapCommand {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Blocks = append([]DocumentAnchorBlockDTO(nil), value.Blocks...)
	return &copy
}
