package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func withImage() Request {
	m := Text("user", "what's in this?")
	m.Images = []Image{{MediaType: "image/png", Data: []byte("PNG")}}
	return Request{Model: "m", Messages: []Message{Text("system", "sys"), m}}
}

func TestImagesOnTheWire(t *testing.T) {
	// Chat completions: content parts with a data URL.
	b, _ := json.Marshal(toChatRequest(withImage()))
	if !strings.Contains(string(b), `"content":[{"type":"text","text":"what's in this?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,UE5H"}}]`) ||
		!strings.Contains(string(b), `"content":"sys"`) {
		t.Fatalf("chat completions: %s", b)
	}
	// Responses API: input_text + input_image.
	b, _ = json.Marshal(toResponsesRequest(withImage()))
	if !strings.Contains(string(b), `{"image_url":"data:image/png;base64,UE5H","type":"input_image"}`) {
		t.Fatalf("responses: %s", b)
	}
	// Anthropic: a base64 image block.
	b, _ = json.Marshal(toAnthropicRequest(withImage()))
	if !strings.Contains(string(b), `{"type":"image","source":{"type":"base64","media_type":"image/png","data":"UE5H"}}`) {
		t.Fatalf("anthropic: %s", b)
	}
	// Images never reach the stored context.
	b, _ = json.Marshal(withImage().Messages[1])
	if strings.Contains(string(b), "UE5H") {
		t.Fatalf("stored message carries image bytes: %s", b)
	}
}

// A model that rejects images gets the request again without them, and isn't sent images again.
func TestModelWithoutVision(t *testing.T) {
	var calls atomic.Int32
	var lastBody atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body)
		lastBody.Store(string(raw))
		if strings.Contains(string(raw), "image_url") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":{"message":"This model does not support image input"}}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": "ok"}}}})
	}))
	defer srv.Close()
	c := New(srv.URL, "openbot/test", func(context.Context) (string, error) { return "k", nil })
	c.RetryDelays = []time.Duration{time.Millisecond}
	if _, err := c.Chat(context.Background(), withImage()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || !strings.Contains(lastBody.Load().(string), "can't see images") {
		t.Fatalf("calls %d, last body %s", calls.Load(), lastBody.Load())
	}
	calls.Store(0)
	if _, err := c.Chat(context.Background(), withImage()); err != nil || calls.Load() != 1 {
		t.Fatalf("second request should skip images straight away: %d calls, %v", calls.Load(), err)
	}
}
