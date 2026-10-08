package model

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

	c := New(srv.URL, "openbot/1.2.3", func(context.Context) (string, error) { return "secret", nil })
	if _, err := c.Chat(context.Background(), Request{Session: "openbot-agent-abc", Model: "m"}); err != nil {
		t.Fatal(err)
	}
	for header, want := range map[string]string{
		"Authorization":      "Bearer secret",
		"User-Agent":         "openbot/1.2.3",
		"X-Opencode-Session": "openbot-agent-abc",
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
		if !strings.HasPrefix(r.Header.Get("X-Opencode-Session"), "openbot-verify-") {
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

	c := New(srv.URL, "openbot/test", nil)
	if err := c.VerifyKey(context.Background(), "good"); err != nil {
		t.Fatalf("good key: %v", err)
	}
	if err := c.VerifyKey(context.Background(), "bad"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("bad key: expected ErrInvalidKey, got %v", err)
	}
}

// Temporary provider failures are retried a few times; real errors aren't.
func TestChatRetriesTransientFailures(t *testing.T) {
	var calls atomic.Int32
	status := http.StatusServiceUnavailable
	failures := int32(2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= failures {
			w.WriteHeader(status)
			w.Write([]byte(`{"error":{"message":"The backend is temporarily overloaded. Please retry."}}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "hi"}}},
		})
	}))
	defer srv.Close()
	c := New(srv.URL, "openbot/test", func(context.Context) (string, error) { return "k", nil })
	c.RetryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

	if _, err := c.Chat(context.Background(), Request{Model: "m"}); err != nil || calls.Load() != 3 {
		t.Fatalf("two 503s then success: err %v after %d calls", err, calls.Load())
	}

	calls.Store(0)
	failures = 10
	_, err := c.Chat(context.Background(), Request{Model: "m"})
	var api *APIError
	if !errors.As(err, &api) || api.Status != 503 || calls.Load() != 4 {
		t.Fatalf("gives up after 3 retries: err %v after %d calls", err, calls.Load())
	}

	calls.Store(0)
	status = http.StatusBadRequest
	if _, err := c.Chat(context.Background(), Request{Model: "m"}); err == nil || calls.Load() != 1 {
		t.Fatalf("a 400 isn't retried: %d calls", calls.Load())
	}
}

func TestWindows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"opencode":{"models":{"kimi":{"limit":{"context":200000}},"glm":{"limit":{"context":64000}}}},
			"opencode-go":{"models":{"kimi":{"limit":{"context":262144}}}},"other":{"models":{"x":{"limit":{"context":5}}}}}`))
	}))
	defer srv.Close()
	w := NewWindows(srv.URL)
	w.Refresh()
	if w.Get("kimi") != 262144 || w.Get("glm") != 64000 || w.Get("x") != 0 {
		t.Fatalf("windows: kimi=%d glm=%d x=%d", w.Get("kimi"), w.Get("glm"), w.Get("x"))
	}
}
