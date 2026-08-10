package textstructure

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

func TestExtractorProducesVersionedBilingualStructuredRecallKeys(t *testing.T) {
	t.Parallel()

	extractor := NewExtractor()
	plaintext := "OpenAI announced the launch in San Francisco, United States. 人工智能公司在北京发布新产品。"
	result, err := extractor.ExtractDocumentStructure(context.Background(), ingestionapplication.ExtractDocumentStructureCommand{
		DocumentVersionID: 71,
		ContentSHA256:     fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext))),
		Title:             "OpenAI launches GPT-5 in San Francisco",
		Plaintext:         plaintext,
		Language:          "en",
	})
	if err != nil {
		t.Fatalf("ExtractDocumentStructure() error = %v", err)
	}
	if result.ProfileVersion != ingestionapplication.CanonicalDocumentStructureProfileVersion {
		t.Fatalf("profile = %q", result.ProfileVersion)
	}
	for name, test := range map[string]struct {
		got  []string
		want []string
	}{
		"actions":   {result.ActionKeys, []string{"announce", "launch", "release"}},
		"locations": {result.LocationKeys, []string{"beijing", "san francisco", "united states"}},
		"regions":   {result.RegionKeys, []string{"china", "us"}},
	} {
		if !reflect.DeepEqual(test.got, test.want) {
			t.Fatalf("%s = %#v, want %#v", name, test.got, test.want)
		}
	}
	for _, entity := range []string{"openai", "gpt-5", "openai launches", "人工智能"} {
		if !containsKey(result.EntityKeys, entity) {
			t.Fatalf("entity keys %#v do not contain %q", result.EntityKeys, entity)
		}
	}
	if containsKey(result.EntityKeys, "#") {
		t.Fatalf("punctuation-only token leaked into entity keys: %#v", result.EntityKeys)
	}
}

func TestExtractorRecognizesOptimizationAsAnEventAction(t *testing.T) {
	t.Parallel()
	extractor := NewExtractor()
	plaintext := "PBTune provides evolutionary auto-tuning for PostgreSQL without an external ML service."
	result, err := extractor.ExtractDocumentStructure(context.Background(), ingestionapplication.ExtractDocumentStructureCommand{
		DocumentVersionID: 73, ContentSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext))),
		Title: "PBTune: evolutionary auto-tuning for PostgreSQL", Plaintext: plaintext, Language: "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(result.ActionKeys, "optimize") || containsKey(result.EntityKeys, "#") {
		t.Fatalf("structured keys = %#v / %#v", result.EntityKeys, result.ActionKeys)
	}
}

func TestExtractorIsDeterministicBoundedAndFailClosed(t *testing.T) {
	t.Parallel()

	extractor := NewExtractor()
	plaintext := "某公司在上海宣布收购目标公司。"
	command := ingestionapplication.ExtractDocumentStructureCommand{
		DocumentVersionID: 72, ContentSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext))),
		Title: "「某公司」宣布收购", Plaintext: plaintext, Language: "zh-CN",
	}
	first, err := extractor.ExtractDocumentStructure(context.Background(), command)
	if err != nil {
		t.Fatalf("first extraction error = %v", err)
	}
	second, err := extractor.ExtractDocumentStructure(context.Background(), command)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("deterministic extraction = %#v / %#v / %v", first, second, err)
	}
	if len(first.EntityKeys) > ingestionapplication.MaximumDocumentStructureKeys || !containsKey(first.ActionKeys, "acquire") || !containsKey(first.LocationKeys, "shanghai") {
		t.Fatalf("bounded Chinese extraction = %#v", first)
	}
	if _, err := extractor.ExtractDocumentStructure(context.Background(), ingestionapplication.ExtractDocumentStructureCommand{DocumentVersionID: 1, ContentSHA256: strings.Repeat("a", 64), Title: "x", Plaintext: "bad\rbody", Language: "en"}); err == nil || !errors.Is(err, sharedrepository.ErrInvalidInput) {
		t.Fatalf("invalid canonical plaintext error = %v", err)
	}
}

func containsKey(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
