package model

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatSendsProviderHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "hi"}}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "barn/1.2.3", func(context.Context) (string, error) { return "secret", nil })
	if _, err := c.Chat(context.Background(), Request{Session: "barn-agent-abc", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	for header, want := range map[string]string{
		"Authorization":      "Bearer secret",
		"User-Agent":         "barn/1.2.3",
		"X-Opencode-Session": "barn-agent-abc",
	} {
		if got.Get(header) != want {
			t.Errorf("%s = %q, want %q", header, got.Get(header), want)
		}
	}
}

func TestVerifyKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "big"}, {"id": "small-flash"}}})
			return
		}
		if !strings.HasPrefix(r.Header.Get("X-Opencode-Session"), "barn-verify-") {
			http.Error(w, `{"error":{"message":"Request is missing x-opencode-session"}}`, http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req Request
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "small-flash" || req.MaxTokens != 1 {
			t.Errorf("expected a 1-token request on the cheap model, got %+v", req)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "o"}}},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "barn/test", nil)
	if err := c.VerifyKey(context.Background(), "good"); err != nil {
		t.Fatalf("good key: %v", err)
	}
	if err := c.VerifyKey(context.Background(), "bad"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("bad key: expected ErrInvalidKey, got %v", err)
	}
}
