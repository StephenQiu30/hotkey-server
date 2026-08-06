package domain

import "math"

const EmbeddingDimensions = 1024

func ValidateEmbedding(vector []float32) error {
	if len(vector) != EmbeddingDimensions {
		return NewError(CodeAIEmbeddingInvalid)
	}
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return NewError(CodeAIEmbeddingInvalid)
		}
	}
	return nil
}
