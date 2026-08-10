package domain

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	CanonicalTextQuoteNormalizationVersion = "nfc-lf-collapse-space-v1"
	CanonicalTextQuoteSelectorVersion      = "w3c-text-quote-position-nfc-utf8-v1"
	MaximumExactQuoteBytes                 = 4096
	maximumTextQuoteContextRunes           = 64
)

type TextQuoteSelectorCandidate struct {
	ExactQuote           string
	Prefix               string
	Suffix               string
	UTF8ByteStart        int64
	UTF8ByteEnd          int64
	PlaintextSHA256      string
	NormalizationVersion string
}

type TextQuoteSelector struct {
	ExactQuote           string
	Prefix               string
	Suffix               string
	UTF8ByteStart        int64
	UTF8ByteEnd          int64
	QuoteSHA256          string
	PlaintextSHA256      string
	NormalizationVersion string
	SelectorVersion      string
}

func BuildTextQuoteSelector(plaintext string, candidate TextQuoteSelectorCandidate) (TextQuoteSelector, error) {
	if plaintext == "" || !utf8.ValidString(plaintext) || strings.ContainsRune(plaintext, '\r') || norm.NFC.String(plaintext) != plaintext ||
		candidate.NormalizationVersion != CanonicalTextQuoteNormalizationVersion ||
		candidate.PlaintextSHA256 != textQuoteSHA256(plaintext) ||
		!validTextQuoteRange(plaintext, candidate.UTF8ByteStart, candidate.UTF8ByteEnd) {
		return TextQuoteSelector{}, errors.New("text quote plaintext identity or range is invalid")
	}
	exact := plaintext[candidate.UTF8ByteStart:candidate.UTF8ByteEnd]
	if exact == "" || len(exact) > MaximumExactQuoteBytes || exact != candidate.ExactQuote || !canonicalTextQuoteValue(exact) {
		return TextQuoteSelector{}, errors.New("text quote exact value does not match the immutable plaintext range")
	}
	prefix, suffix := CanonicalTextQuoteContext(plaintext, candidate.UTF8ByteStart, candidate.UTF8ByteEnd)
	if candidate.Prefix != prefix || candidate.Suffix != suffix || !canonicalTextQuoteValue(prefix) || !canonicalTextQuoteValue(suffix) {
		return TextQuoteSelector{}, errors.New("text quote context does not match the immutable plaintext")
	}
	return TextQuoteSelector{
		ExactQuote: exact, Prefix: prefix, Suffix: suffix,
		UTF8ByteStart: candidate.UTF8ByteStart, UTF8ByteEnd: candidate.UTF8ByteEnd,
		QuoteSHA256: textQuoteSHA256(exact), PlaintextSHA256: candidate.PlaintextSHA256,
		NormalizationVersion: candidate.NormalizationVersion, SelectorVersion: CanonicalTextQuoteSelectorVersion,
	}, nil
}

func CanonicalTextQuoteContext(plaintext string, start, end int64) (string, string) {
	if !validTextQuoteRange(plaintext, start, end) {
		return "", ""
	}
	before := []rune(plaintext[:start])
	after := []rune(plaintext[end:])
	if len(before) > maximumTextQuoteContextRunes {
		before = before[len(before)-maximumTextQuoteContextRunes:]
	}
	if len(after) > maximumTextQuoteContextRunes {
		after = after[:maximumTextQuoteContextRunes]
	}
	return string(before), string(after)
}

func ValidateTextQuoteSelector(selector TextQuoteSelector) error {
	if selector.ExactQuote == "" || len(selector.ExactQuote) > MaximumExactQuoteBytes ||
		!canonicalTextQuoteValue(selector.ExactQuote) || !canonicalTextQuoteValue(selector.Prefix) || !canonicalTextQuoteValue(selector.Suffix) ||
		selector.UTF8ByteStart < 0 || selector.UTF8ByteEnd <= selector.UTF8ByteStart ||
		selector.QuoteSHA256 != textQuoteSHA256(selector.ExactQuote) || !validDocumentSHA256(selector.PlaintextSHA256) ||
		selector.NormalizationVersion != CanonicalTextQuoteNormalizationVersion || selector.SelectorVersion != CanonicalTextQuoteSelectorVersion {
		return errors.New("text quote selector is invalid")
	}
	return nil
}

func validTextQuoteRange(value string, start, end int64) bool {
	if !utf8.ValidString(value) || start < 0 || end <= start || end > int64(len(value)) {
		return false
	}
	return textQuoteRuneBoundary(value, start) && textQuoteRuneBoundary(value, end)
}

func textQuoteRuneBoundary(value string, offset int64) bool {
	return offset == 0 || offset == int64(len(value)) || offset > 0 && offset < int64(len(value)) && utf8.RuneStart(value[offset])
}

func canonicalTextQuoteValue(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, '\r') && norm.NFC.String(value) == value
}

func textQuoteSHA256(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}
