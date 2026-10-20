package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"goguide/internal/domain"
)

func TestEinoIntentExtraction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "chatcmpl-test", "object": "chat.completion", "created": 1, "model": "test-model", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": `{"budget_cents":800000,"categories":["laptop","monitor","dock"],"use_case":"编程","require_one_cable":true}`}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_MODEL", "test-model")
	t.Setenv("OPENAI_BASE_URL", server.URL+"/v1")
	g := NewGuide(nil)
	if g.Mode != "eino" || g.InitErr != nil {
		t.Fatalf("model setup: %s %v", g.Mode, g.InitErr)
	}
	intent, err := g.extract(context.Background(), "预算8000元，买笔记本、显示器和扩展坞", domain.Intent{LockedSKUs: []int64{7}})
	if err != nil {
		t.Fatal(err)
	}
	if intent.BudgetCents != 800000 || len(intent.Categories) != 3 || len(intent.LockedSKUs) != 1 {
		t.Fatalf("unexpected intent %+v", intent)
	}
}

func TestUnknownHardConditionFailsClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": `{"budget_cents":800000,"categories":["laptop","monitor"],"hard":["必须触屏"]}`}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("OPENAI_MODEL", "test-model")
	t.Setenv("OPENAI_BASE_URL", server.URL+"/v1")
	intent, err := NewGuide(nil).extract(context.Background(), "预算 8000 元，买笔记本和显示器，必须触屏", domain.Intent{})
	if err != nil {
		t.Fatal(err)
	}
	if len(intent.Hard) == 0 {
		t.Fatal("unsupported hard condition was dropped")
	}
}

func TestGuideRequiresModel(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	g := NewGuide(nil)
	if g.Mode != "unavailable" || g.Model != nil {
		t.Fatalf("guide should be unavailable without model: %+v", g)
	}
	if _, err := g.extract(context.Background(), "预算1W买笔记本加显示器", domain.Intent{}); err == nil {
		t.Fatal("guide silently accepted request without model")
	}
}
