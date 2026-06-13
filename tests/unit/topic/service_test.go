package topic_test

import (
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/topic"
)

func TestClusterPostsCreatesSingleTopicForSimilarPosts(t *testing.T) {
	svc := topic.NewService(nil)
	topics := svc.Cluster([]topic.CandidatePost{
		{PostID: 1, Tokens: []string{"openai", "agent", "launch"}},
		{PostID: 2, Tokens: []string{"openai", "agent", "release"}},
	})
	if len(topics) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(topics))
	}
}

func TestClusterPostsCreatesSeparateTopicsForDissimilarPosts(t *testing.T) {
	svc := topic.NewService(nil)
	topics := svc.Cluster([]topic.CandidatePost{
		{PostID: 1, Tokens: []string{"openai", "agent", "launch"}},
		{PostID: 2, Tokens: []string{"cooking", "recipe", "pasta"}},
	})
	if len(topics) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(topics))
	}
}

func TestClusterPostsEmptyInput(t *testing.T) {
	svc := topic.NewService(nil)
	topics := svc.Cluster([]topic.CandidatePost{})
	if len(topics) != 0 {
		t.Fatalf("expected 0 topics, got %d", len(topics))
	}
}

func TestClusterPostsSinglePost(t *testing.T) {
	svc := topic.NewService(nil)
	topics := svc.Cluster([]topic.CandidatePost{
		{PostID: 1, Tokens: []string{"ai", "ml"}},
	})
	if len(topics) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(topics))
	}
	if topics[0].Title == "" {
		t.Fatal("expected non-empty topic title")
	}
	if len(topics[0].PostIDs) != 1 {
		t.Fatalf("expected 1 post in topic, got %d", len(topics[0].PostIDs))
	}
}

func TestClusterPostsGroupsByOverlap(t *testing.T) {
	svc := topic.NewService(nil)
	topics := svc.Cluster([]topic.CandidatePost{
		{PostID: 1, Tokens: []string{"ai", "gpt", "openai"}},
		{PostID: 2, Tokens: []string{"ai", "gpt", "launch"}},
		{PostID: 3, Tokens: []string{"crypto", "bitcoin", "eth"}},
		{PostID: 4, Tokens: []string{"crypto", "bitcoin", "defi"}},
	})
	if len(topics) != 2 {
		t.Fatalf("expected 2 topics, got %d", len(topics))
	}
}

func TestComputeJaccardSimilarity(t *testing.T) {
	a := []string{"ai", "gpt", "openai"}
	b := []string{"ai", "gpt", "launch"}
	sim := topic.JaccardSimilarity(a, b)
	// intersection: {ai, gpt} = 2, union: {ai, gpt, openai, launch} = 4
	expected := 2.0 / 4.0
	if sim != expected {
		t.Fatalf("expected %f, got %f", expected, sim)
	}
}

func TestComputeJaccardSimilarityIdentical(t *testing.T) {
	a := []string{"ai", "gpt"}
	b := []string{"ai", "gpt"}
	sim := topic.JaccardSimilarity(a, b)
	if sim != 1.0 {
		t.Fatalf("expected 1.0, got %f", sim)
	}
}

func TestComputeJaccardSimilarityDisjoint(t *testing.T) {
	a := []string{"ai"}
	b := []string{"cooking"}
	sim := topic.JaccardSimilarity(a, b)
	if sim != 0 {
		t.Fatalf("expected 0, got %f", sim)
	}
}

func TestExtractTokens(t *testing.T) {
	tokens := topic.ExtractTokens("OpenAI launches new Agent framework")
	// Should lowercase and split
	if len(tokens) == 0 {
		t.Fatal("expected non-empty tokens")
	}
	for _, tok := range tokens {
		if tok == "" {
			t.Fatal("expected no empty tokens")
		}
	}
}
