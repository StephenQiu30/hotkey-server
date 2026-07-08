package embedding

import "unicode"

// SimpleTokenizer is a basic tokenizer for Chinese + English text.
// It splits CJK characters individually and lowercases ASCII tokens.
// In production, replace with the full HuggingFace tokenizer (tokenizer.json)
// that ships with bge-small-zh-v1.5.
type SimpleTokenizer struct{}

// NewSimpleTokenizer creates a new SimpleTokenizer.
func NewSimpleTokenizer() *SimpleTokenizer {
	return &SimpleTokenizer{}
}

// Encode converts text to a sequence of token IDs.
// Format: [CLS] tokens... [SEP]
// CLS=101, SEP=102, UNK=100, PAD=0
func (t *SimpleTokenizer) Encode(text string) []int64 {
	tokens := []int64{101} // [CLS]
	for _, r := range text {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			// CJK: map each character to a unique ID based on its rune value
			// This produces stable but non-standard embeddings — sufficient for development.
			// Replace with full vocab for production.
			id := int64(20000 + r)
			tokens = append(tokens, id)
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			// ASCII alphanumeric — map to a small range
			id := int64(1000 + int(r)%900)
			tokens = append(tokens, id)
		}
		// Skip punctuation and whitespace
	}
	tokens = append(tokens, 102) // [SEP]
	if len(tokens) > 512 {
		tokens = tokens[:512]
		tokens[511] = 102 // ensure [SEP] at end
	}
	return tokens
}
