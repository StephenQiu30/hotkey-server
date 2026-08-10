package domain

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestBuildTextQuoteSelectorUsesNFCUTF8ByteOffsetsAndCanonicalContext(t *testing.T) {
	t.Parallel()
	plaintext := "开场。\n\nCafé 发布新模型，性能提升。\n\n结尾。"
	exact := "Café 发布新模型"
	start := int64(len("开场。\n\n"))
	end := start + int64(len(exact))
	selector, err := BuildTextQuoteSelector(plaintext, TextQuoteSelectorCandidate{
		ExactQuote: exact, Prefix: "开场。\n\n", Suffix: "，性能提升。\n\n结尾。",
		UTF8ByteStart: start, UTF8ByteEnd: end,
		PlaintextSHA256:      digestTextQuoteTest(plaintext),
		NormalizationVersion: CanonicalTextQuoteNormalizationVersion,
	})
	if err != nil {
		t.Fatalf("BuildTextQuoteSelector() error = %v", err)
	}
	if selector.ExactQuote != exact || selector.UTF8ByteStart != start || selector.UTF8ByteEnd != end ||
		selector.QuoteSHA256 != digestTextQuoteTest(exact) || selector.SelectorVersion != CanonicalTextQuoteSelectorVersion {
		t.Fatalf("selector = %#v", selector)
	}
}

func TestBuildTextQuoteSelectorRejectsDriftAndMultibyteBoundaryCuts(t *testing.T) {
	t.Parallel()
	plaintext := "甲乙 Café 丙丁"
	base := TextQuoteSelectorCandidate{
		ExactQuote: "Café", Prefix: "甲乙 ", Suffix: " 丙丁",
		UTF8ByteStart: int64(len("甲乙 ")), UTF8ByteEnd: int64(len("甲乙 Café")),
		PlaintextSHA256: digestTextQuoteTest(plaintext), NormalizationVersion: CanonicalTextQuoteNormalizationVersion,
	}
	tests := []struct {
		name   string
		mutate func(*TextQuoteSelectorCandidate)
	}{
		{name: "exact drift", mutate: func(value *TextQuoteSelectorCandidate) { value.ExactQuote = "Cafe" }},
		{name: "prefix drift", mutate: func(value *TextQuoteSelectorCandidate) { value.Prefix = "乙 " }},
		{name: "suffix drift", mutate: func(value *TextQuoteSelectorCandidate) { value.Suffix = "丙丁" }},
		{name: "plaintext digest drift", mutate: func(value *TextQuoteSelectorCandidate) { value.PlaintextSHA256 = digestTextQuoteTest("other") }},
		{name: "multibyte start", mutate: func(value *TextQuoteSelectorCandidate) { value.UTF8ByteStart++ }},
		{name: "multibyte end", mutate: func(value *TextQuoteSelectorCandidate) { value.UTF8ByteEnd-- }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.mutate(&candidate)
			if _, err := BuildTextQuoteSelector(plaintext, candidate); err == nil {
				t.Fatal("invalid selector was accepted")
			}
		})
	}
}

func TestBuildTextQuoteSelectorBoundsCanonicalContext(t *testing.T) {
	t.Parallel()
	prefix := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789前后"
	suffix := "后续abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	plaintext := prefix + "引用" + suffix
	start := int64(len(prefix))
	end := start + int64(len("引用"))
	wantPrefix, wantSuffix := CanonicalTextQuoteContext(plaintext, start, end)
	selector, err := BuildTextQuoteSelector(plaintext, TextQuoteSelectorCandidate{
		ExactQuote: "引用", Prefix: wantPrefix, Suffix: wantSuffix,
		UTF8ByteStart: start, UTF8ByteEnd: end, PlaintextSHA256: digestTextQuoteTest(plaintext),
		NormalizationVersion: CanonicalTextQuoteNormalizationVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if selector.Prefix != wantPrefix || selector.Suffix != wantSuffix || len([]rune(selector.Prefix)) > 64 || len([]rune(selector.Suffix)) > 64 {
		t.Fatalf("bounded context = %#v", selector)
	}
}

func digestTextQuoteTest(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}
