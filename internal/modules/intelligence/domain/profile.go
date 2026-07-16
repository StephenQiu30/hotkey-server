package domain

import (
	"fmt"
	"math/big"
	"strings"
)

// ModelProfile contains immutable provider/semantic identity plus the few
// operational knobs that later application services may update under version
// control. Amounts remain decimal strings to preserve PostgreSQL numeric(12,4)
// precision without introducing a floating-point budget boundary.
type ModelProfile struct {
	ID, Version                                   int64
	Name                                          string
	TaskType                                      TaskType
	Provider                                      ProviderName
	ModelName, ModelVersion                       string
	CredentialRef                                 *string
	EmbeddingDimensions                           *int
	TimeoutSeconds, MaxAttempts, FallbackPriority int
	MaxCost                                       string
	DailyBudget                                   *string
	Enabled                                       bool
}

func (profile ModelProfile) Validate() error {
	if strings.TrimSpace(profile.Name) == "" || !profile.TaskType.Valid() || !profile.Provider.Valid() ||
		strings.TrimSpace(profile.ModelName) == "" || strings.TrimSpace(profile.ModelVersion) == "" ||
		profile.TimeoutSeconds <= 0 || profile.MaxAttempts < 1 || profile.MaxAttempts > 3 || profile.FallbackPriority < 0 {
		return NewError(CodeAIModelProfileInvalid)
	}
	maxCost, err := positiveCost(profile.MaxCost)
	if err != nil {
		return NewError(CodeAIModelProfileInvalid)
	}
	if profile.DailyBudget != nil {
		dailyBudget, err := positiveCost(*profile.DailyBudget)
		if err != nil || dailyBudget.Cmp(maxCost) < 0 {
			return NewError(CodeAIModelProfileInvalid)
		}
	}
	switch profile.Provider {
	case ProviderOpenAI:
		if profile.CredentialRef == nil || *profile.CredentialRef != OpenAICredentialReference {
			return NewError(CodeAIModelProfileInvalid)
		}
	case ProviderONNX:
		if profile.CredentialRef != nil || profile.TaskType != TaskTypeEmbedding {
			return NewError(CodeAIModelProfileInvalid)
		}
	}
	if profile.TaskType == TaskTypeEmbedding {
		if profile.EmbeddingDimensions == nil || *profile.EmbeddingDimensions != EmbeddingDimensions {
			return NewError(CodeAIModelProfileInvalid)
		}
	} else if profile.EmbeddingDimensions != nil {
		return NewError(CodeAIModelProfileInvalid)
	}
	return nil
}

// SameSemanticIdentity identifies the fields that are not mutable in place.
// Operational changes such as timeout, attempts, budgets, priority and enabled
// state intentionally do not alter model/vector provenance.
func (profile ModelProfile) SameSemanticIdentity(other ModelProfile) bool {
	return profile.TaskType == other.TaskType && profile.Provider == other.Provider &&
		profile.ModelName == other.ModelName && profile.ModelVersion == other.ModelVersion &&
		stringPointerEqual(profile.CredentialRef, other.CredentialRef) && intPointerEqual(profile.EmbeddingDimensions, other.EmbeddingDimensions)
}

func positiveCost(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "eE+-") || strings.Count(value, ".") > 1 {
		return nil, fmt.Errorf("cost must be a positive decimal")
	}
	parts := strings.Split(value, ".")
	if len(parts) == 2 && len(parts[1]) > 4 {
		return nil, fmt.Errorf("cost exceeds four decimal places")
	}
	for _, part := range parts {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return nil, fmt.Errorf("cost must be decimal digits")
		}
	}
	rational, ok := new(big.Rat).SetString(value)
	if !ok || rational.Sign() <= 0 {
		return nil, fmt.Errorf("cost must be positive")
	}
	return rational, nil
}

func stringPointerEqual(first, second *string) bool {
	return first == nil && second == nil || first != nil && second != nil && *first == *second
}

func intPointerEqual(first, second *int) bool {
	return first == nil && second == nil || first != nil && second != nil && *first == *second
}
