package domain

import (
	"math"
	"testing"
)

func TestValidateEmbeddingRequiresExactly1024FiniteValues(t *testing.T) {
	valid := make([]float32, EmbeddingDimensions)
	valid[0] = 1
	if err := ValidateEmbedding(valid); err != nil {
		t.Fatalf("ValidateEmbedding() error = %v", err)
	}

	for _, test := range []struct {
		name   string
		vector []float32
	}{
		{"short", make([]float32, EmbeddingDimensions-1)},
		{"long", make([]float32, EmbeddingDimensions+1)},
		{"NaN", vectorWith(0, float32(math.NaN()))},
		{"Inf", vectorWith(0, float32(math.Inf(1)))},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateEmbedding(test.vector); err == nil {
				t.Fatal("ValidateEmbedding() error = nil, want rejection")
			} else if code, ok := CodeOf(err); !ok || code != CodeAIEmbeddingInvalid {
				t.Fatalf("ValidateEmbedding() code = %d/%t, want %d", code, ok, CodeAIEmbeddingInvalid)
			}
		})
	}
}

func vectorWith(index int, value float32) []float32 {
	vector := make([]float32, EmbeddingDimensions)
	vector[index] = value
	return vector
}
