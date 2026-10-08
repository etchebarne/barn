package model

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// ModelsDevURL lists models' limits, including OpenCode Go's (provider "opencode-go").
const ModelsDevURL = "https://models.dev/api.json"

// Windows knows models' context windows, from models.dev. It refreshes in the background, so
// Get never waits on the network; unknown models (or before the first fetch) report 0.
type Windows struct {
	url  string
	http *http.Client

	mu      sync.Mutex
	windows map[string]int
	fetched time.Time
	loading bool
}

func NewWindows(url string) *Windows {
	return &Windows{url: url, http: &http.Client{Timeout: 30 * time.Second}, windows: map[string]int{}}
}

// Get returns a model's context window in tokens (0 if unknown).
func (w *Windows) Get(model string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.loading && time.Since(w.fetched) > 24*time.Hour {
		w.loading = true
		go w.refresh()
	}
	return w.windows[model]
}

// Refresh fetches the list now (e.g. at startup).
func (w *Windows) Refresh() {
	w.mu.Lock()
	w.loading = true
	w.mu.Unlock()
	w.refresh()
}

func (w *Windows) refresh() {
	windows, err := w.fetch()
	w.mu.Lock()
	defer w.mu.Unlock()
	w.loading = false
	if err != nil {
		slog.Warn("couldn't load model context windows", "err", err)
		w.fetched = time.Now().Add(-23 * time.Hour) // try again in an hour
		return
	}
	w.windows, w.fetched = windows, time.Now()
}

func (w *Windows) fetch() (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := w.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var providers map[string]struct {
		Models map[string]struct {
			Limit struct {
				Context int `json:"context"`
			} `json:"limit"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&providers); err != nil {
		return nil, err
	}
	out := map[string]int{}
	// OpenCode Go's own entries win over OpenCode Zen's.
	for _, name := range []string{"opencode", "opencode-go"} {
		for id, m := range providers[name].Models {
			if m.Limit.Context > 0 {
				out[id] = m.Limit.Context
			}
		}
	}
	return out, nil
}
