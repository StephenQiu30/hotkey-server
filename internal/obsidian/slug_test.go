package obsidian

import "testing"

func TestSlugify_RemovesSpecialChars(t *testing.T) {
	got := Slugify("AI 监管/政策!")
	want := "ai-监管-政策"
	if got != want {
		t.Fatalf("Slugify(%q) = %q, want %q", "AI 监管/政策!", got, want)
	}
}

func TestSlugify_Chinese(t *testing.T) {
	got := Slugify("大模型安全与伦理")
	want := "大模型安全与伦理"
	if got != want {
		t.Fatalf("Slugify(%q) = %q, want %q", "大模型安全与伦理", got, want)
	}
}

func TestSlugify_Empty(t *testing.T) {
	got := Slugify("")
	if got != "" {
		t.Fatalf("Slugify(\"\") = %q, want %q", got, "")
	}
}

func TestSlugify_OnlySpecialChars(t *testing.T) {
	got := Slugify("/!@#$%")
	if got != "" {
		t.Fatalf("Slugify(%q) = %q, want %q", "/!@#$%", got, "")
	}
}

func TestSlugify_MixedAlphanumericAndChinese(t *testing.T) {
	got := Slugify("GPT-4 评测 2026!")
	want := "gpt-4-评测-2026"
	if got != want {
		t.Fatalf("Slugify(%q) = %q, want %q", "GPT-4 评测 2026!", got, want)
	}
}

func TestSlugify_CollapsesMultipleDashes(t *testing.T) {
	got := Slugify("hello---world")
	want := "hello-world"
	if got != want {
		t.Fatalf("Slugify(%q) = %q, want %q", "hello---world", got, want)
	}
}

func TestSlugify_TrimsDashes(t *testing.T) {
	got := Slugify("--hello--")
	want := "hello"
	if got != want {
		t.Fatalf("Slugify(%q) = %q, want %q", "--hello--", got, want)
	}
}
