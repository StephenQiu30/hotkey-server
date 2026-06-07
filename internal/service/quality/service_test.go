package quality

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/domain/content"
)

func TestScoreFullQualityItem(t *testing.T) {
	svc := NewService(DefaultConfig())
	result, err := svc.Score(context.Background(), content.SourceItem{
		Title:        "人工智能最新进展：深度学习技术取得重大突破",
		Snippet:      "研究人员在自然语言处理领域取得了重大突破，新的模型在多项基准测试中表现优异，涵盖文本分类、情感分析、机器翻译和问答系统等多项核心任务，推动了整个行业的技术进步与创新发展。",
		Language:     "zh",
		CanonicalURL: "https://example.com/article/1",
	})
	if err != nil {
		t.Fatalf("score failed: %v", err)
	}
	if result.Score < 0.8 {
		t.Fatalf("expected high score for full quality item, got %f", result.Score)
	}
	if !result.Summarizable {
		t.Fatal("expected summarizable for full quality item")
	}
}

func TestScoreLowQualityItemMissingContent(t *testing.T) {
	svc := NewService(DefaultConfig())
	result, err := svc.Score(context.Background(), content.SourceItem{
		Title:        "短",
		Snippet:      "",
		Language:     "unknown",
		CanonicalURL: "",
	})
	if err != nil {
		t.Fatalf("score failed: %v", err)
	}
	if result.Score > 0.3 {
		t.Fatalf("expected low score for poor item, got %f", result.Score)
	}
	if result.Summarizable {
		t.Fatal("expected not summarizable for poor item")
	}
}

func TestScoreMarksSummarizableWhenSufficientContent(t *testing.T) {
	svc := NewService(DefaultConfig())
	result, err := svc.Score(context.Background(), content.SourceItem{
		Title:    "AI 新闻标题",
		Snippet:  "这是一段足够长的摘要内容用于判断是否可以生成摘要",
		Language: "zh",
	})
	if err != nil {
		t.Fatalf("score failed: %v", err)
	}
	if !result.Summarizable {
		t.Fatal("expected summarizable when content is sufficient")
	}
}

func TestScoreMarksNotSummarizableWhenTooShort(t *testing.T) {
	svc := NewService(DefaultConfig())
	result, err := svc.Score(context.Background(), content.SourceItem{
		Title:    "短",
		Snippet:  "太短",
		Language: "zh",
	})
	if err != nil {
		t.Fatalf("score failed: %v", err)
	}
	if result.Summarizable {
		t.Fatal("expected not summarizable when content is too short")
	}
}

func TestScorePenalizesUnknownLanguage(t *testing.T) {
	svc := NewService(DefaultConfig())
	withLang, err := svc.Score(context.Background(), content.SourceItem{
		Title:    "人工智能最新进展",
		Snippet:  "研究人员在自然语言处理领域取得了重大突破",
		Language: "zh",
	})
	if err != nil {
		t.Fatalf("score failed: %v", err)
	}
	noLang, err := svc.Score(context.Background(), content.SourceItem{
		Title:    "人工智能最新进展",
		Snippet:  "研究人员在自然语言处理领域取得了重大突破",
		Language: "unknown",
	})
	if err != nil {
		t.Fatalf("score failed: %v", err)
	}
	if noLang.Score >= withLang.Score {
		t.Fatalf("expected unknown language to score lower: with=%f, without=%f", withLang.Score, noLang.Score)
	}
}

func TestScoreReturnsNonNegative(t *testing.T) {
	svc := NewService(DefaultConfig())
	result, err := svc.Score(context.Background(), content.SourceItem{})
	if err != nil {
		t.Fatalf("score failed: %v", err)
	}
	if result.Score < 0 {
		t.Fatalf("expected non-negative score, got %f", result.Score)
	}
}
