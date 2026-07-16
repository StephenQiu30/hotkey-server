package domain

import "testing"

func TestModelProfileValidatesOnlyPlan008SemanticCombinations(t *testing.T) {
	profile := validOpenAIEmbeddingProfile()
	if err := profile.Validate(); err != nil {
		t.Fatalf("ModelProfile.Validate() error = %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*ModelProfile)
	}{
		{"future task", func(profile *ModelProfile) { profile.TaskType = TaskType("summary") }},
		{"unsupported provider", func(profile *ModelProfile) { profile.Provider = ProviderName("anthropic") }},
		{"OpenAI credential", func(profile *ModelProfile) { value := "env:OTHER"; profile.CredentialRef = &value }},
		{"ONNX term expansion", func(profile *ModelProfile) {
			profile.Provider, profile.CredentialRef, profile.TaskType = ProviderONNX, nil, TaskTypeTermExpansion
			profile.EmbeddingDimensions = nil
		}},
		{"embedding without dimension", func(profile *ModelProfile) { profile.EmbeddingDimensions = nil }},
		{"invalid max cost", func(profile *ModelProfile) { profile.MaxCost = "0" }},
		{"daily below max", func(profile *ModelProfile) { value := "0.9999"; profile.DailyBudget = &value }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := validOpenAIEmbeddingProfile()
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("ModelProfile.Validate() error = nil, want rejection")
			}
		})
	}
}

func TestModelProfileSemanticIdentityExcludesOnlyOperationalFields(t *testing.T) {
	profile := validOpenAIEmbeddingProfile()
	operational := profile
	operational.Enabled = false
	operational.TimeoutSeconds = 45
	if !profile.SameSemanticIdentity(operational) {
		t.Fatal("SameSemanticIdentity() changed after only operational fields")
	}

	semantic := profile
	semantic.ModelVersion = "2026-08"
	if profile.SameSemanticIdentity(semantic) {
		t.Fatal("SameSemanticIdentity() accepted changed model version")
	}
}

func validOpenAIEmbeddingProfile() ModelProfile {
	credential := OpenAICredentialReference
	dimension := EmbeddingDimensions
	dailyBudget := "5.0000"
	return ModelProfile{
		ID:                  1,
		Version:             1,
		Name:                "embedding-primary",
		TaskType:            TaskTypeEmbedding,
		Provider:            ProviderOpenAI,
		ModelName:           "text-embedding-3-large",
		ModelVersion:        "2026-07",
		CredentialRef:       &credential,
		EmbeddingDimensions: &dimension,
		TimeoutSeconds:      30,
		MaxAttempts:         2,
		MaxCost:             "1.0000",
		DailyBudget:         &dailyBudget,
		FallbackPriority:    100,
		Enabled:             true,
	}
}
