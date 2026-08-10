package application

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestTextQuoteSelectorServiceVerifiesExactPlaintextAndPersistsRightsBoundSelector(t *testing.T) {
	t.Parallel()
	plaintext := "第一段。\n\nCafé 发布新模型，性能提升。\n\n第三段。"
	plaintextSHA := textQuoteDigest(plaintext)
	start := int64(len("第一段。\n\n"))
	end := start + int64(len("Café 发布新模型"))
	prefix, suffix := canonicalQuoteContextForTest(plaintext, start, end)
	now := time.Date(2026, 8, 10, 16, 0, 0, 0, time.UTC)
	repository := &textQuoteSelectorRepositoryFake{target: TextQuoteSelectorTargetDTO{
		SourceConnectionID: 7, DocumentID: 11, DocumentVersionID: 13, ContentSHA256: plaintextSHA,
		DocumentLifecycleState: DocumentReadable,
		PlaintextArtifact: TextQuoteProjectionArtifactDTO{ID: 17, ArtifactType: DocumentProjectionPlaintext,
			TransformerProfileSHA256: strings.Repeat("a", 64), MIMEType: "text/plain; charset=utf-8",
			SHA256: plaintextSHA, SizeBytes: int64(len(plaintext)), RetentionUntil: now.Add(24 * time.Hour)},
		MarkdownArtifactID: 19, AnchorMapSHA256: strings.Repeat("b", 64),
		AnchorBlocks: []TextQuoteAnchorBlockDTO{{Ordinal: 0, PlaintextUTF8ByteStart: 0, PlaintextUTF8ByteEnd: int64(len("第一段。")), MarkdownAnchor: "body-0000-111111111111"},
			{Ordinal: 1, PlaintextUTF8ByteStart: start, PlaintextUTF8ByteEnd: int64(len("第一段。\n\nCafé 发布新模型，性能提升。")), MarkdownAnchor: "body-0001-222222222222"}},
		QuoteRightsDecisionID: 23, RetainRightsDecisionID: 29, RetentionUntil: now.Add(24 * time.Hour), DecisionAt: now,
	}}
	projection := &textQuoteProjectionReaderFake{result: knowledgeapplication.DocumentProjectionResultDTO{
		Content: plaintext, MIMEType: "text/plain; charset=utf-8", SHA256: plaintextSHA, SizeBytes: int64(len(plaintext)),
	}}
	repository.result = TextQuoteSelectorDTO{ID: 31, Version: 1, SourceConnectionID: 7, DocumentVersionID: 13,
		PlaintextArtifactID: 17, MarkdownArtifactID: 19, QuoteRightsDecisionID: 23, RetainRightsDecisionID: 29,
		ExactQuote: "Café 发布新模型", Prefix: prefix, Suffix: suffix, UTF8ByteStart: start, UTF8ByteEnd: end,
		QuoteSHA256: textQuoteDigest("Café 发布新模型"), PlaintextSHA256: plaintextSHA,
		NormalizationVersion: CanonicalDocumentTextNormalizationVersion, SelectorVersion: CanonicalDocumentTextQuoteSelectorVersion,
		AnchorMapSHA256: strings.Repeat("b", 64), MarkdownAnchor: pointerToQuoteString("body-0001-222222222222"),
		RetentionUntil: now.Add(24 * time.Hour), CreatedAt: now,
	}
	service, err := NewTextQuoteSelectorService(TextQuoteSelectorDependencies{Repository: repository, Projections: projection})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Create(context.Background(), CreateTextQuoteSelectorCommand{
		DocumentVersionID: 13, ExactQuote: "Café 发布新模型", Prefix: prefix, Suffix: suffix,
		UTF8ByteStart: start, UTF8ByteEnd: end, PlaintextSHA256: plaintextSHA,
		NormalizationVersion: CanonicalDocumentTextNormalizationVersion, DecisionAt: now,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Selector.ID != 31 || result.Selector.MarkdownAnchor == nil || *result.Selector.MarkdownAnchor != "body-0001-222222222222" {
		t.Fatalf("Create() = %#v", result)
	}
	if projection.query.DocumentID != 11 || projection.query.DocumentVersionID != 13 || projection.query.ArtifactType != "plaintext" ||
		projection.query.SHA256 != plaintextSHA || projection.query.MaxBytes != MaximumCanonicalSourceBodyBytes {
		t.Fatalf("projection query = %#v", projection.query)
	}
	if repository.command.QuoteRightsDecisionID != 23 || repository.command.RetainRightsDecisionID != 29 ||
		repository.command.ExactQuote != "Café 发布新模型" || repository.command.MarkdownAnchor == nil {
		t.Fatalf("persist command = %#v", repository.command)
	}
}

func TestTextQuoteSelectorServiceFailsBeforePersistenceOnDriftOrMissingQuoteRights(t *testing.T) {
	t.Parallel()
	plaintext := "甲乙 Café 丙丁"
	digest := textQuoteDigest(plaintext)
	start, end := int64(len("甲乙 ")), int64(len("甲乙 Café"))
	prefix, suffix := canonicalQuoteContextForTest(plaintext, start, end)
	now := time.Now().UTC()
	baseTarget := TextQuoteSelectorTargetDTO{
		SourceConnectionID: 7, DocumentID: 11, DocumentVersionID: 13, ContentSHA256: digest, DocumentLifecycleState: DocumentReadable,
		PlaintextArtifact: TextQuoteProjectionArtifactDTO{ID: 17, ArtifactType: "plaintext", TransformerProfileSHA256: strings.Repeat("a", 64),
			MIMEType: "text/plain; charset=utf-8", SHA256: digest, SizeBytes: int64(len(plaintext)), RetentionUntil: now.Add(time.Hour)},
		QuoteRightsDecisionID: 23, RetainRightsDecisionID: 29, RetentionUntil: now.Add(time.Hour), DecisionAt: now,
	}
	command := CreateTextQuoteSelectorCommand{DocumentVersionID: 13, ExactQuote: "Café", Prefix: prefix, Suffix: suffix,
		UTF8ByteStart: start, UTF8ByteEnd: end, PlaintextSHA256: digest,
		NormalizationVersion: CanonicalDocumentTextNormalizationVersion, DecisionAt: now}
	tests := []struct {
		name   string
		mutate func(*TextQuoteSelectorTargetDTO, *knowledgeapplication.DocumentProjectionResultDTO, *CreateTextQuoteSelectorCommand)
	}{
		{name: "quote rights absent", mutate: func(target *TextQuoteSelectorTargetDTO, _ *knowledgeapplication.DocumentProjectionResultDTO, _ *CreateTextQuoteSelectorCommand) {
			target.QuoteRightsDecisionID = 0
		}},
		{name: "vault digest drift", mutate: func(_ *TextQuoteSelectorTargetDTO, result *knowledgeapplication.DocumentProjectionResultDTO, _ *CreateTextQuoteSelectorCommand) {
			result.SHA256 = strings.Repeat("f", 64)
		}},
		{name: "exact quote drift", mutate: func(_ *TextQuoteSelectorTargetDTO, _ *knowledgeapplication.DocumentProjectionResultDTO, command *CreateTextQuoteSelectorCommand) {
			command.ExactQuote = "Cafe"
		}},
		{name: "multibyte boundary", mutate: func(_ *TextQuoteSelectorTargetDTO, _ *knowledgeapplication.DocumentProjectionResultDTO, command *CreateTextQuoteSelectorCommand) {
			command.UTF8ByteEnd--
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := baseTarget
			projectionResult := knowledgeapplication.DocumentProjectionResultDTO{Content: plaintext, MIMEType: "text/plain; charset=utf-8", SHA256: digest, SizeBytes: int64(len(plaintext))}
			input := command
			test.mutate(&target, &projectionResult, &input)
			repository := &textQuoteSelectorRepositoryFake{target: target}
			service, _ := NewTextQuoteSelectorService(TextQuoteSelectorDependencies{Repository: repository, Projections: &textQuoteProjectionReaderFake{result: projectionResult}})
			if _, err := service.Create(context.Background(), input); err == nil || repository.persistCalls != 0 {
				t.Fatalf("Create() error/calls = %v/%d", err, repository.persistCalls)
			}
		})
	}
}

func TestTextQuoteSelectorServiceLocatesUniqueMultibyteQuoteAndRejectsAmbiguity(t *testing.T) {
	t.Parallel()
	plaintext := "第一段。\n\nCafé 发布新模型。\n\n第三段。"
	plaintextSHA := textQuoteDigest(plaintext)
	now := time.Date(2026, 8, 10, 18, 0, 0, 0, time.UTC)
	start := int64(len("第一段。\n\n"))
	end := start + int64(len("Café 发布新模型"))
	prefix, suffix := canonicalQuoteContextForTest(plaintext, start, end)
	repository := &textQuoteSelectorRepositoryFake{target: TextQuoteSelectorTargetDTO{
		SourceConnectionID: 7, DocumentID: 11, DocumentVersionID: 13, ContentSHA256: plaintextSHA,
		DocumentLifecycleState: DocumentReadable,
		PlaintextArtifact: TextQuoteProjectionArtifactDTO{ID: 17, ArtifactType: DocumentProjectionPlaintext,
			TransformerProfileSHA256: strings.Repeat("a", 64), MIMEType: "text/plain; charset=utf-8",
			SHA256: plaintextSHA, SizeBytes: int64(len(plaintext)), RetentionUntil: now.Add(time.Hour)},
		MarkdownArtifactID: 19, AnchorMapSHA256: strings.Repeat("b", 64),
		AnchorBlocks: []TextQuoteAnchorBlockDTO{{Ordinal: 1, PlaintextUTF8ByteStart: start,
			PlaintextUTF8ByteEnd: end + int64(len("。")), MarkdownAnchor: "body-0001-222222222222"}},
		QuoteRightsDecisionID: 23, RetainRightsDecisionID: 29,
		RetentionUntil: now.Add(time.Hour), DecisionAt: now,
	}}
	repository.result = TextQuoteSelectorDTO{ID: 31, Version: 1, SourceConnectionID: 7, DocumentVersionID: 13,
		PlaintextArtifactID: 17, MarkdownArtifactID: 19, QuoteRightsDecisionID: 23, RetainRightsDecisionID: 29,
		ExactQuote: "Café 发布新模型", Prefix: prefix, Suffix: suffix, UTF8ByteStart: start, UTF8ByteEnd: end,
		QuoteSHA256: textQuoteDigest("Café 发布新模型"), PlaintextSHA256: plaintextSHA,
		NormalizationVersion: CanonicalDocumentTextNormalizationVersion, SelectorVersion: CanonicalDocumentTextQuoteSelectorVersion,
		AnchorMapSHA256: strings.Repeat("b", 64), MarkdownAnchor: pointerToQuoteString("body-0001-222222222222"),
		RetentionUntil: now.Add(time.Hour), CreatedAt: now,
	}
	reader := &textQuoteProjectionReaderFake{result: knowledgeapplication.DocumentProjectionResultDTO{
		Content: plaintext, MIMEType: "text/plain; charset=utf-8", SHA256: plaintextSHA, SizeBytes: int64(len(plaintext)),
	}}
	service, _ := NewTextQuoteSelectorService(TextQuoteSelectorDependencies{Repository: repository, Projections: reader})
	result, err := service.LocateAndCreate(context.Background(), LocateTextQuoteSelectorCommand{
		DocumentVersionID: 13, ExactQuote: "Café 发布新模型", PlaintextSHA256: plaintextSHA,
		NormalizationVersion: CanonicalDocumentTextNormalizationVersion, DecisionAt: now,
	})
	if err != nil || result.Selector.ID != 31 || repository.command.UTF8ByteStart != start || repository.command.UTF8ByteEnd != end {
		t.Fatalf("LocateAndCreate() = %#v / %v command=%#v", result, err, repository.command)
	}

	ambiguous := "重复引用。重复引用。"
	ambiguousSHA := textQuoteDigest(ambiguous)
	repository.target.ContentSHA256 = ambiguousSHA
	repository.target.PlaintextArtifact.SHA256 = ambiguousSHA
	repository.target.PlaintextArtifact.SizeBytes = int64(len(ambiguous))
	reader.result = knowledgeapplication.DocumentProjectionResultDTO{Content: ambiguous, MIMEType: "text/plain; charset=utf-8", SHA256: ambiguousSHA, SizeBytes: int64(len(ambiguous))}
	repository.persistCalls = 0
	_, err = service.LocateAndCreate(context.Background(), LocateTextQuoteSelectorCommand{
		DocumentVersionID: 13, ExactQuote: "重复引用", PlaintextSHA256: ambiguousSHA,
		NormalizationVersion: CanonicalDocumentTextNormalizationVersion, DecisionAt: now,
	})
	if err == nil || repository.persistCalls != 0 {
		t.Fatalf("ambiguous LocateAndCreate() error/calls = %v/%d", err, repository.persistCalls)
	}
}

type textQuoteSelectorRepositoryFake struct {
	target       TextQuoteSelectorTargetDTO
	result       TextQuoteSelectorDTO
	command      PersistTextQuoteSelectorCommand
	persistCalls int
}

func (fake *textQuoteSelectorRepositoryFake) ReadTextQuoteSelectorTarget(_ context.Context, query TextQuoteSelectorTargetQuery) (TextQuoteSelectorTargetDTO, error) {
	if query.DocumentVersionID <= 0 {
		return TextQuoteSelectorTargetDTO{}, sharedrepository.ErrInvalidInput
	}
	return fake.target, nil
}

func (fake *textQuoteSelectorRepositoryFake) PersistTextQuoteSelector(_ context.Context, command PersistTextQuoteSelectorCommand) (TextQuoteSelectorDTO, error) {
	fake.persistCalls++
	fake.command = command
	return fake.result, nil
}

type textQuoteProjectionReaderFake struct {
	query  knowledgeapplication.DocumentProjectionQueryDTO
	result knowledgeapplication.DocumentProjectionResultDTO
}

func (fake *textQuoteProjectionReaderFake) ReadDocumentProjection(_ context.Context, query knowledgeapplication.DocumentProjectionQueryDTO) (knowledgeapplication.DocumentProjectionResultDTO, error) {
	fake.query = query
	return fake.result, nil
}

func textQuoteDigest(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func canonicalQuoteContextForTest(plaintext string, start, end int64) (string, string) {
	return canonicalTextQuoteContext(plaintext, start, end)
}

func pointerToQuoteString(value string) *string { return &value }
