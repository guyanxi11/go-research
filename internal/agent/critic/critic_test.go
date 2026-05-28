package critic

import (
	"strings"
	"testing"
)

func TestParseReview_Pass(t *testing.T) {
	r, err := parseReview(`{"score": 8, "feedback": ""}`, 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Score != 8 {
		t.Errorf("score = %d, want 8", r.Score)
	}
	if !r.Pass {
		t.Errorf("Pass = false, want true (8 >= 6)")
	}
}

func TestParseReview_Fail_WithFeedback(t *testing.T) {
	r, err := parseReview(`{"score": 4, "feedback": "  cite more sources  "}`, 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Pass {
		t.Errorf("Pass = true, want false (4 < 6)")
	}
	if r.Feedback != "cite more sources" {
		t.Errorf("Feedback = %q, want trimmed", r.Feedback)
	}
}

func TestParseReview_ClampLow(t *testing.T) {
	r, err := parseReview(`{"score": -5, "feedback": "x"}`, 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Score != 1 {
		t.Errorf("Score = %d, want clamped to 1", r.Score)
	}
}

func TestParseReview_ClampHigh(t *testing.T) {
	r, err := parseReview(`{"score": 42, "feedback": ""}`, 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Score != 10 {
		t.Errorf("Score = %d, want clamped to 10", r.Score)
	}
}

func TestParseReview_NoisyLLMOutput(t *testing.T) {
	// Real LLMs frequently wrap JSON in chatter or ```json fences.
	raw := "Sure, here is my review:\n```json\n{\"score\": 7, \"feedback\": \"thin on metrics\"}\n```\nLet me know!"
	r, err := parseReview(raw, 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Score != 7 || !r.Pass || r.Feedback != "thin on metrics" {
		t.Errorf("unexpected review: %+v", r)
	}
}

func TestParseReview_NoJSON(t *testing.T) {
	_, err := parseReview("the answer is great, no JSON here", 6)
	if err == nil {
		t.Fatal("expected error for missing JSON")
	}
	if !strings.Contains(err.Error(), "no JSON") {
		t.Errorf("error = %v, want it to mention missing JSON", err)
	}
}

func TestParseReview_DefaultMinScore(t *testing.T) {
	// minScore <= 0 should fall back to 6 (matches New() default).
	r, err := parseReview(`{"score": 6}`, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Pass {
		t.Errorf("Pass = false, want true at default threshold 6")
	}
}
