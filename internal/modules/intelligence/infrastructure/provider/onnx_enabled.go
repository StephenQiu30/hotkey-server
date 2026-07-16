//go:build onnx && cgo

package provider

import (
	"context"
	"errors"
	"math"
	"sync"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	ort "github.com/yalue/onnxruntime_go"
)

var onnxRuntimeMu sync.Mutex

// ONNXProvider is a local embedding-only provider. Its constructor completes
// all artifact and model-contract validation before any inference is possible.
type ONNXProvider struct {
	manifest  onnxManifest
	tokenizer *onnxTokenizer
	session   *ort.DynamicAdvancedSession
	mu        sync.Mutex
}

func NewONNXProvider(ai config.AIConfig) (intelligencedomain.Provider, error) {
	provider, err := newONNXProvider(ai)
	if err != nil {
		return nil, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	return provider, nil
}

func newONNXProvider(ai config.AIConfig) (*ONNXProvider, error) {
	if err := requireRegularFile(ai.ONNXRuntimeLibrary); err != nil {
		return nil, err
	}
	if err := requireRegularFile(ai.ONNXModelPath); err != nil {
		return nil, err
	}
	if err := requireRegularFile(ai.ONNXTokenizerPath); err != nil {
		return nil, err
	}
	manifest, err := loadONNXManifest(ai.ONNXManifestPath)
	if err != nil {
		return nil, err
	}
	if err := verifyArtifactSHA256(ai.ONNXModelPath, manifest.ModelSHA256); err != nil {
		return nil, err
	}
	if err := verifyArtifactSHA256(ai.ONNXTokenizerPath, manifest.TokenizerSHA256); err != nil {
		return nil, err
	}
	tokenizer, err := loadONNXTokenizer(ai.ONNXTokenizerPath, manifest.MaxTokens)
	if err != nil {
		return nil, err
	}

	onnxRuntimeMu.Lock()
	defer onnxRuntimeMu.Unlock()
	if !ort.IsInitialized() {
		ort.SetSharedLibraryPath(ai.ONNXRuntimeLibrary)
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, err
		}
	}
	inputs, outputs, err := ort.GetInputOutputInfo(ai.ONNXModelPath)
	if err != nil {
		return nil, err
	}
	if !matchesONNXContract(inputs, outputs, manifest) {
		return nil, errors.New("ONNX tensor contract mismatch")
	}
	session, err := ort.NewDynamicAdvancedSession(ai.ONNXModelPath, manifest.InputNames, []string{manifest.OutputName}, nil)
	if err != nil {
		return nil, err
	}
	return &ONNXProvider{manifest: manifest, tokenizer: tokenizer, session: session}, nil
}

func (provider *ONNXProvider) Embed(ctx context.Context, request intelligencedomain.EmbeddingRequest) (intelligencedomain.EmbeddingResponse, error) {
	if provider == nil {
		return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}
	if err := request.Validate(); err != nil {
		return intelligencedomain.EmbeddingResponse{}, err
	}
	if !provider.supportsEmbedding(request) || provider.session == nil || provider.tokenizer == nil {
		return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
	}

	vectors := make([][]float32, len(request.Inputs))
	for index, input := range request.Inputs {
		if err := ctx.Err(); err != nil {
			return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIProviderTimeout)
		}
		vector, err := provider.embedOne(input)
		if err != nil {
			return intelligencedomain.EmbeddingResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
		}
		vectors[index] = vector
	}
	return intelligencedomain.EmbeddingResponse{ModelVersion: request.ModelVersion, Vectors: vectors}, nil
}

func (provider *ONNXProvider) supportsEmbedding(request intelligencedomain.EmbeddingRequest) bool {
	return request.ModelVersion == provider.manifest.ModelVersion && request.Dimensions == provider.manifest.Dimensions
}

func (provider *ONNXProvider) GenerateStructured(context.Context, intelligencedomain.StructuredRequest) (intelligencedomain.StructuredResponse, error) {
	return intelligencedomain.StructuredResponse{}, intelligencedomain.NewError(intelligencedomain.CodeAIModelUnavailable)
}

func (provider *ONNXProvider) embedOne(input string) ([]float32, error) {
	ids, attentionMask, tokenTypeIDs, err := provider.tokenizer.Encode(input)
	if err != nil {
		return nil, err
	}
	shape := ort.NewShape(1, int64(provider.manifest.MaxTokens))
	inputIDs, err := ort.NewTensor(shape, ids)
	if err != nil {
		return nil, err
	}
	defer inputIDs.Destroy()
	attention, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return nil, err
	}
	defer attention.Destroy()
	tokenTypes, err := ort.NewTensor(shape, tokenTypeIDs)
	if err != nil {
		return nil, err
	}
	defer tokenTypes.Destroy()

	provider.mu.Lock()
	defer provider.mu.Unlock()
	outputs := []ort.Value{nil}
	if err := provider.session.Run([]ort.Value{inputIDs, attention, tokenTypes}, outputs); err != nil {
		return nil, err
	}
	if outputs[0] == nil {
		return nil, errors.New("ONNX session returned no output")
	}
	defer outputs[0].Destroy()
	output, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, errors.New("ONNX output is not float32")
	}
	return clsL2(output.GetData(), output.GetShape(), provider.manifest.Dimensions)
}

func matchesONNXContract(inputs, outputs []ort.InputOutputInfo, manifest onnxManifest) bool {
	if len(inputs) != len(manifest.InputNames) {
		return false
	}
	inputByName := make(map[string]ort.InputOutputInfo, len(inputs))
	for _, input := range inputs {
		inputByName[input.Name] = input
	}
	for _, name := range manifest.InputNames {
		input, ok := inputByName[name]
		if !ok || input.OrtValueType != ort.ONNXTypeTensor || input.DataType != ort.TensorElementDataTypeInt64 || len(input.Dimensions) != 2 {
			return false
		}
		if input.Dimensions[1] > 0 && input.Dimensions[1] != int64(manifest.MaxTokens) {
			return false
		}
	}
	for _, output := range outputs {
		if output.Name == manifest.OutputName {
			return output.OrtValueType == ort.ONNXTypeTensor && output.DataType == ort.TensorElementDataTypeFloat && len(output.Dimensions) == 3 && output.Dimensions[2] == int64(manifest.Dimensions)
		}
	}
	return false
}

func clsL2(data []float32, shape ort.Shape, dimensions int) ([]float32, error) {
	if len(shape) != 3 || shape[0] != 1 || shape[2] != int64(dimensions) || len(data) < dimensions {
		return nil, errors.New("unexpected ONNX output shape")
	}
	vector := append([]float32(nil), data[:dimensions]...)
	var squaredNorm float64
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return nil, errors.New("non-finite ONNX output")
		}
		squaredNorm += float64(value) * float64(value)
	}
	if squaredNorm == 0 || math.IsNaN(squaredNorm) || math.IsInf(squaredNorm, 0) {
		return nil, errors.New("invalid ONNX output norm")
	}
	norm := math.Sqrt(squaredNorm)
	for index := range vector {
		vector[index] /= float32(norm)
	}
	if err := intelligencedomain.ValidateEmbedding(vector); err != nil {
		return nil, err
	}
	return vector, nil
}

var _ intelligencedomain.Provider = (*ONNXProvider)(nil)
