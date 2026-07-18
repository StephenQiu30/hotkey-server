package domain

import (
	"strings"
	"testing"
)

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

func TestPlan018ProviderProfileContracts(t *testing.T) {
	deepSeekCredential := DeepSeekCredentialReference
	deepSeek := validOpenAIEmbeddingProfile()
	deepSeek.Name = "deepseek-summary"
	deepSeek.TaskType = TaskTypeEventSummary
	deepSeek.Provider = ProviderDeepSeek
	deepSeek.ModelName = "deepseek-v4-pro"
	deepSeek.CredentialRef = &deepSeekCredential
	deepSeek.EmbeddingDimensions = nil
	if err := deepSeek.Validate(); err != nil {
		t.Fatalf("DeepSeek profile Validate() error = %v", err)
	}

	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	dimensions := EmbeddingDimensions
	ollama := deepSeek
	ollama.Name = "ollama-qwen-embedding"
	ollama.TaskType = TaskTypeEmbedding
	ollama.Provider = ProviderOllama
	ollama.ModelName = OllamaQwenEmbeddingModel
	ollama.ModelVersion = digest
	ollama.CredentialRef = nil
	ollama.EmbeddingDimensions = &dimensions
	if err := ollama.Validate(); err != nil {
		t.Fatalf("Ollama Qwen profile Validate() error = %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*ModelProfile)
	}{
		{"DeepSeek embedding", func(profile *ModelProfile) {
			profile.TaskType, profile.EmbeddingDimensions = TaskTypeEmbedding, &dimensions
		}},
		{"DeepSeek wrong credential", func(profile *ModelProfile) { value := "env:OPENAI_API_KEY"; profile.CredentialRef = &value }},
		{"Ollama credential", func(profile *ModelProfile) { value := "env:OLLAMA_KEY"; profile.CredentialRef = &value }},
		{"Ollama mutable version", func(profile *ModelProfile) { profile.ModelVersion = "latest" }},
		{"Ollama wrong embedding model", func(profile *ModelProfile) { profile.ModelName = "qwen3-embedding:4b" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := deepSeek
			if strings.HasPrefix(test.name, "Ollama") {
				candidate = ollama
			}
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("ModelProfile.Validate() error = nil, want rejection")
			}
		})
	}
}

func TestPlan009RelevanceReviewContract(t *testing.T) {
	profile := validOpenAIEmbeddingProfile()
	profile.Name = "relevance-review-primary"
	profile.TaskType = TaskTypeRelevanceReview
	profile.ModelName = "gpt-5.6sol"
	profile.EmbeddingDimensions = nil
	if err := profile.Validate(); err != nil {
		t.Fatalf("relevance-review profile Validate() error = %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*ModelProfile)
	}{
		{"onnx is unavailable", func(profile *ModelProfile) {
			profile.Provider, profile.CredentialRef = ProviderONNX, nil
		}},
		{"embedding dimensions are forbidden", func(profile *ModelProfile) {
			dimensions := EmbeddingDimensions
			profile.EmbeddingDimensions = &dimensions
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := profile
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("relevance-review profile Validate() error = nil, want rejection")
			}
		})
	}
}

func TestPlan012EventIntelligenceProfileContracts(t *testing.T) {
	for _, taskType := range []TaskType{TaskTypeEventSummary, TaskTypeEntityClaimExtraction} {
		profile := validOpenAIEmbeddingProfile()
		profile.Name = string(taskType) + "-primary"
		profile.TaskType = taskType
		profile.ModelName = "gpt-5.6sol"
		profile.EmbeddingDimensions = nil
		if err := profile.Validate(); err != nil {
			t.Fatalf("%s profile Validate() error = %v", taskType, err)
		}
		profile.Provider, profile.CredentialRef = ProviderONNX, nil
		if err := profile.Validate(); err == nil {
			t.Fatalf("%s ONNX profile Validate() error = nil, want rejection", taskType)
		}
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
