//go:build onnx && cgo

package provider

import (
	"errors"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
	"golang.org/x/text/unicode/norm"
)

// onnxTokenizer owns the validated HuggingFace tokenizer.json contract. The
// third-party parser remains inside the native adapter build and never crosses
// the provider boundary.
type onnxTokenizer struct {
	native    *tokenizer.Tokenizer
	maxTokens int
	padID     int
}

func loadONNXTokenizer(path string, maxTokens int) (*onnxTokenizer, error) {
	if maxTokens <= 0 {
		return nil, errors.New("invalid ONNX tokenizer maximum length")
	}
	native, err := pretrained.FromFile(path)
	if err != nil {
		return nil, err
	}
	padID, ok := tokenizerPaddingID(native)
	if !ok {
		return nil, errors.New("tokenizer does not define a padding token")
	}
	return &onnxTokenizer{native: native, maxTokens: maxTokens, padID: padID}, nil
}

func (tokenizer *onnxTokenizer) Encode(input string) (ids, attentionMask, tokenTypeIDs []int64, err error) {
	if tokenizer == nil || tokenizer.native == nil {
		return nil, nil, nil, errors.New("ONNX tokenizer is unavailable")
	}
	encoding, err := tokenizer.native.EncodeSingle(norm.NFC.String(input), true)
	if err != nil || encoding == nil || len(encoding.Ids) == 0 {
		if err != nil {
			return nil, nil, nil, err
		}
		return nil, nil, nil, errors.New("tokenizer produced no tokens")
	}

	ids = make([]int64, tokenizer.maxTokens)
	attentionMask = make([]int64, tokenizer.maxTokens)
	tokenTypeIDs = make([]int64, tokenizer.maxTokens)
	for index := range ids {
		ids[index] = int64(tokenizer.padID)
	}
	limit := len(encoding.Ids)
	if limit > tokenizer.maxTokens {
		limit = tokenizer.maxTokens
	}
	for index := 0; index < limit; index++ {
		if encoding.Ids[index] < 0 {
			return nil, nil, nil, errors.New("tokenizer produced a negative token ID")
		}
		ids[index] = int64(encoding.Ids[index])
		attentionMask[index] = 1
		if index < len(encoding.TypeIds) {
			tokenTypeIDs[index] = int64(encoding.TypeIds[index])
		}
	}
	return ids, attentionMask, tokenTypeIDs, nil
}

func tokenizerPaddingID(tokenizer *tokenizer.Tokenizer) (int, bool) {
	for _, token := range []string{"[PAD]", "<pad>", "<PAD>"} {
		if id, ok := tokenizer.TokenToId(token); ok {
			return id, true
		}
	}
	return 0, false
}
