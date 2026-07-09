package service

import (
	"context"
	"fmt"
	"math"
	"unicode"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/StephenQiu30/hotkey-server/internal/pkg"
)

// Embedder is the ONNX model interface for generating text embeddings.
type Embedder interface {
	Embed(tokenIDs []int64) ([384]float32, error)
	Close() error
}

// Tokenizer converts text to token IDs suitable for the embedding model.
type Tokenizer interface {
	Encode(text string) []int64
}

// EmbeddingModel wraps an ONNX runtime session for text embedding inference.
type EmbeddingModel struct {
	session *ort.AdvancedSession
	inputT  *ort.Tensor[int64]
	maskT   *ort.Tensor[int64]
	outputT *ort.Tensor[float32]
}

// NewEmbeddingModel loads an ONNX embedding model from the given file path.
func NewEmbeddingModel(modelPath string) (*EmbeddingModel, error) {
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("onnx init: %w", err)
	}

	seqLen := int64(512)
	inputShape := ort.NewShape(1, seqLen)
	inputT, err := ort.NewEmptyTensor[int64](inputShape)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create input tensor: %w", err)
	}

	maskShape := ort.NewShape(1, seqLen)
	maskT, err := ort.NewEmptyTensor[int64](maskShape)
	if err != nil {
		inputT.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create mask tensor: %w", err)
	}

	outputShape := ort.NewShape(1, 384)
	outputT, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		inputT.Destroy()
		maskT.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create output tensor: %w", err)
	}

	inputNames := []string{"input_ids", "attention_mask"}
	outputNames := []string{"sentence_embedding"}

	session, err := ort.NewAdvancedSession(
		modelPath,
		inputNames,
		outputNames,
		[]ort.ArbitraryTensor{inputT, maskT},
		[]ort.ArbitraryTensor{outputT},
		nil,
	)
	if err != nil {
		inputT.Destroy()
		maskT.Destroy()
		outputT.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &EmbeddingModel{session: session, inputT: inputT, maskT: maskT, outputT: outputT}, nil
}

// Embed runs inference on the given token IDs and returns the embedding vector.
func (m *EmbeddingModel) Embed(tokenIDs []int64) ([384]float32, error) {
	var result [384]float32

	seqLen := len(tokenIDs)
	if seqLen > 512 {
		seqLen = 512
	}

	inputData := make([]int64, 512)
	maskData := make([]int64, 512)
	for i := 0; i < seqLen; i++ {
		inputData[i] = tokenIDs[i]
		maskData[i] = 1
	}

	copy(m.inputT.GetData(), inputData)
	copy(m.maskT.GetData(), maskData)

	if err := m.session.Run(); err != nil {
		return result, fmt.Errorf("onnx run: %w", err)
	}

	copy(result[:], m.outputT.GetData())
	return result, nil
}

// Close releases the ONNX session and environment.
func (m *EmbeddingModel) Close() error {
	var errs []error
	if err := m.session.Destroy(); err != nil {
		errs = append(errs, err)
	}
	m.inputT.Destroy()
	m.maskT.Destroy()
	m.outputT.Destroy()
	ort.DestroyEnvironment()
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// SimpleTokenizer is a basic tokenizer for Chinese + English text.
type SimpleTokenizer struct{}

// NewSimpleTokenizer creates a new SimpleTokenizer.
func NewSimpleTokenizer() *SimpleTokenizer {
	return &SimpleTokenizer{}
}

// Encode converts text to a sequence of token IDs.
func (t *SimpleTokenizer) Encode(text string) []int64 {
	tokens := []int64{101} // [CLS]
	for _, r := range text {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			id := int64(20000 + r)
			tokens = append(tokens, id)
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			id := int64(1000 + int(r)%900)
			tokens = append(tokens, id)
		}
	}
	tokens = append(tokens, 102) // [SEP]
	if len(tokens) > 512 {
		tokens = tokens[:512]
		tokens[511] = 102
	}
	return tokens
}

// EmbeddingService provides text-to-embedding conversion.
type EmbeddingService struct {
	model     Embedder
	tokenizer Tokenizer
}

// NewEmbeddingService creates an embedding service with the given model and tokenizer.
func NewEmbeddingService(model Embedder, tokenizer Tokenizer) *EmbeddingService {
	return &EmbeddingService{model: model, tokenizer: tokenizer}
}

// NewEmbeddingServiceWithTokenizer creates a service using the default tokenizer.
func NewEmbeddingServiceWithTokenizer(model Embedder) *EmbeddingService {
	return NewEmbeddingService(model, NewSimpleTokenizer())
}

// Embed converts a single text to a normalized 384-dim Vector384.
func (s *EmbeddingService) Embed(ctx context.Context, text string) (pkg.Vector384, error) {
	tokenIDs := s.tokenizer.Encode(text)
	raw, err := s.model.Embed(tokenIDs)
	if err != nil {
		return pkg.Vector384{}, fmt.Errorf("embed: %w", err)
	}
	return l2Normalize(raw), nil
}

// EmbedBatch converts multiple texts to normalized vectors.
func (s *EmbeddingService) EmbedBatch(ctx context.Context, texts []string) ([]pkg.Vector384, error) {
	result := make([]pkg.Vector384, len(texts))
	for i, text := range texts {
		v, err := s.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embed batch [%d]: %w", i, err)
		}
		result[i] = v
	}
	return result, nil
}

// Close releases the underlying model resources.
func (s *EmbeddingService) Close() error {
	return s.model.Close()
}

// l2Normalize applies L2 normalization to the raw model output.
func l2Normalize(raw [384]float32) pkg.Vector384 {
	var sum float64
	for _, v := range raw {
		sum += float64(v) * float64(v)
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		return pkg.Vector384{}
	}
	var result pkg.Vector384
	for i, v := range raw {
		result[i] = float32(float64(v) / norm)
	}
	return result
}
