//go:build onnx && cgo

package provider

import (
	"context"
	"math"
	"testing"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
	ort "github.com/yalue/onnxruntime_go"
)

func TestONNXProviderRejectsProfileVersionMismatchBeforeInference(t *testing.T) {
	provider := &ONNXProvider{manifest: onnxManifest{ModelVersion: "bge-m3-v1", Dimensions: intelligencedomain.EmbeddingDimensions}}
	response, err := provider.Embed(context.Background(), intelligencedomain.EmbeddingRequest{
		ModelName:    "bge-m3",
		ModelVersion: "different-version",
		Dimensions:   intelligencedomain.EmbeddingDimensions,
		Inputs:       []string{"fixture input"},
	})
	if response.ModelVersion != "" || response.Vectors != nil {
		t.Fatalf("Embed() response = %#v, want no result", response)
	}
	code, ok := intelligencedomain.CodeOf(err)
	if !ok || code != intelligencedomain.CodeAIModelUnavailable {
		t.Fatalf("Embed() error = %v, want model unavailable", err)
	}
}

func TestONNXContractRejectsWrongTensorNamesBeforeInference(t *testing.T) {
	manifest := onnxManifest{
		InputNames: []string{"input_ids", "attention_mask", "token_type_ids"},
		OutputName: "last_hidden_state",
		Dimensions: intelligencedomain.EmbeddingDimensions,
		MaxTokens:  8192,
	}
	inputs := []ort.InputOutputInfo{
		{Name: "input_ids", OrtValueType: ort.ONNXTypeTensor, DataType: ort.TensorElementDataTypeInt64, Dimensions: ort.Shape{1, 8192}},
		{Name: "attention_mask", OrtValueType: ort.ONNXTypeTensor, DataType: ort.TensorElementDataTypeInt64, Dimensions: ort.Shape{1, 8192}},
		{Name: "position_ids", OrtValueType: ort.ONNXTypeTensor, DataType: ort.TensorElementDataTypeInt64, Dimensions: ort.Shape{1, 8192}},
	}
	outputs := []ort.InputOutputInfo{{Name: "last_hidden_state", OrtValueType: ort.ONNXTypeTensor, DataType: ort.TensorElementDataTypeFloat, Dimensions: ort.Shape{1, 8192, intelligencedomain.EmbeddingDimensions}}}
	if matchesONNXContract(inputs, outputs, manifest) {
		t.Fatal("matchesONNXContract() = true, want wrong tensor name rejection")
	}
}

func TestONNXCLSPoolingProducesFiniteNormalized1024Vector(t *testing.T) {
	data := make([]float32, 3*intelligencedomain.EmbeddingDimensions)
	for index := 0; index < intelligencedomain.EmbeddingDimensions; index++ {
		data[index] = 1
	}
	vector, err := clsL2(data, ort.Shape{1, 3, intelligencedomain.EmbeddingDimensions}, intelligencedomain.EmbeddingDimensions)
	if err != nil {
		t.Fatalf("clsL2() error = %v", err)
	}
	if len(vector) != intelligencedomain.EmbeddingDimensions {
		t.Fatalf("clsL2() vector length = %d, want 1024", len(vector))
	}
	var squaredNorm float64
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			t.Fatalf("clsL2() produced non-finite value %v", value)
		}
		squaredNorm += float64(value) * float64(value)
	}
	if math.Abs(squaredNorm-1) > 1e-5 {
		t.Fatalf("clsL2() squared norm = %v, want 1", squaredNorm)
	}
}
