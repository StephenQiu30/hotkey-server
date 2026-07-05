package aggregator

import (
	"testing"
	"time"
)

func TestExactMatch(t *testing.T) {
	m := DefaultMatcher()
	a := "AI监管新规出台 网信办发布新政策"
	b := "AI监管新规出台 网信办发布新政策"

	result := m.Match(a, b, time.Now(), time.Now())
	if !result.IsMatch {
		t.Fatal("exact match should return IsMatch=true")
	}
	if result.Score < 0.9 {
		t.Fatalf("exact match score = %f, want >= 0.9", result.Score)
	}
}

func TestSimilarMatch(t *testing.T) {
	m := DefaultMatcher()
	a := "AI监管新规出台 网信办发布管理办法"
	b := "AI监管新规 网信办发布人工智能管理规定"

	result := m.Match(a, b, time.Now(), time.Now())
	if !result.IsMatch {
		t.Fatal("similar topics should match")
	}
}

func TestNoMatch(t *testing.T) {
	m := DefaultMatcher()
	a := "AI监管新规出台 网信办"
	b := "国庆假期旅游攻略 热门景点推荐"

	result := m.Match(a, b, time.Now(), time.Now())
	if result.IsMatch {
		t.Fatal("unrelated topics should not match")
	}
}

func TestTimeWindowExcludes(t *testing.T) {
	m := DefaultMatcher()
	a := "AI监管新规"
	b := "AI监管新规"

	// Outside 24h window
	tA := time.Now()
	tB := time.Now().Add(-48 * time.Hour)

	result := m.Match(a, b, tA, tB)
	if result.IsMatch {
		t.Fatal("topics 48h apart should not match")
	}
}

func TestEmptyInput(t *testing.T) {
	m := DefaultMatcher()
	result := m.Match("", "", time.Now(), time.Now())
	if result.IsMatch {
		t.Fatal("empty input should not match")
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []string{"ai", "监管", "新规"}
	b := []string{"ai", "监管", "新规"}

	score := cosineSimilarity(a, b)
	if score < 0.99 {
		t.Fatalf("identical tokens: cosine = %f, want ~1.0", score)
	}
}

func TestCosineSimilarityDifferent(t *testing.T) {
	a := []string{"ai", "监管"}
	b := []string{"国庆", "旅游"}

	score := cosineSimilarity(a, b)
	if score > 0.1 {
		t.Fatalf("different tokens: cosine = %f, want ~0", score)
	}
}

func TestExtractTokens(t *testing.T) {
	tokens := extractTokens("AI监管新规出台，网信办发布！")
	if len(tokens) == 0 {
		t.Fatal("expected non-empty tokens")
	}
}
