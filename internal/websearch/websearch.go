// Package websearch searches the web through SearXNG, a metasearch engine: either a container
// openbot runs itself (no published port; queries go through `docker exec`) or an instance the
// admin points it at. Everything it returns was written by strangers, so it's cleaned of
// characters that hide text from people and only plain http(s) links are kept.
package websearch

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Image is the SearXNG image the managed container runs. Pinned: bump it deliberately.
const Image = "searxng/searxng:2026.10.7-6671d89be"

const container = "openbot-searxng"

// settingsVersion marks the settings the container was created with; bump it when settings
// change so existing containers are recreated.
const settingsVersion = "1"

// settings turns on the JSON API and leaves out what a public instance needs (the rate limiter
// and its Valkey, the image proxy). The secret key is filled in per container.
const settings = `use_default_settings: true
general:
  instance_name: openbot
server:
  secret_key: "%s"
  limiter: false
  image_proxy: false
  public_instance: false
search:
  safe_search: 0
  formats: [html, json]
`

// Query is one search.
type Query struct {
	Text      string
	Category  string // general (default), news, it, science
	TimeRange string // "", day, week, month, year
	Language  string // e.g. "en", "es-AR"; "" = auto
	Page      int    // 1-based; 0 = 1
}

// Result is one hit.
type Result struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Snippet   string `json:"snippet,omitempty"`
	Published string `json:"published,omitempty"`
}

// Results are what a search found.
type Results struct {
	Results     []Result `json:"results"`
	Suggestions []string `json:"suggestions,omitempty"`
	// Failed lists engines that didn't answer, e.g. "duckduckgo (CAPTCHA)".
	Failed []string `json:"failed_engines,omitempty"`
}

// ErrUnavailable means there's no way to search on this server.
var ErrUnavailable = errors.New("web search isn't available on this server")

// SearXNG searches through a SearXNG instance.
type SearXNG struct {
	url    string // external instance; "" = the managed container
	docker string
	client *http.Client

	mu    sync.Mutex
	ready bool
}

// NewExternal searches through the SearXNG instance at baseURL (its JSON format must be enabled).
func NewExternal(baseURL string) *SearXNG {
	return &SearXNG{url: strings.TrimRight(baseURL, "/"), client: &http.Client{Timeout: 20 * time.Second}}
}

// NewManaged runs SearXNG in a container of its own, started on first use. Without a usable
// Docker, Available reports false.
func NewManaged() *SearXNG {
	s := &SearXNG{}
	if path, err := exec.LookPath("docker"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if exec.CommandContext(ctx, path, "version", "--format", "{{.Server.Version}}").Run() == nil {
			s.docker = path
		}
	}
	return s
}

// Available reports whether searches can run.
func (s *SearXNG) Available() bool { return s.url != "" || s.docker != "" }

// Prepare pulls the image ahead of the first search, so that search isn't slowed down by it.
func (s *SearXNG) Prepare(ctx context.Context) error {
	if s.url != "" {
		return nil
	}
	if s.docker == "" {
		return ErrUnavailable
	}
	if _, err := s.run(ctx, "image", "inspect", Image); err == nil {
		return nil
	}
	_, err := s.run(ctx, "pull", Image)
	return err
}

func (s *SearXNG) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, s.docker, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// ensure makes sure the container runs current settings and answers.
func (s *SearXNG) ensure(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ready {
		// Cheap check: it may have been stopped or removed behind our back.
		if out, err := s.run(ctx, "container", "inspect", "--format", "{{.State.Running}}", container); err == nil && strings.TrimSpace(string(out)) == "true" {
			return nil
		}
		s.ready = false
	}
	want := Image + "|" + settingsVersion
	out, err := s.run(ctx, "container", "inspect", "--format", `{{index .Config.Labels "openbot.searxng"}} {{.State.Running}}`, container)
	switch {
	case err != nil:
		if err := s.create(ctx, want); err != nil {
			return err
		}
	default:
		label, running, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
		if label != want {
			_, _ = s.run(ctx, "rm", "-f", container)
			if err := s.create(ctx, want); err != nil {
				return err
			}
		} else if running != "true" {
			if _, err := s.run(ctx, "start", container); err != nil {
				return err
			}
		}
	}
	// Starting takes a few seconds.
	deadline := time.Now().Add(45 * time.Second)
	for {
		if _, err := s.run(ctx, "exec", container, "wget", "-qO-", "-T", "3", "http://127.0.0.1:8080/healthz"); err == nil {
			s.ready = true
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("the search engine didn't start in time")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (s *SearXNG) create(ctx context.Context, label string) error {
	if err := s.Prepare(ctx); err != nil {
		return fmt.Errorf("getting the search engine: %w", err)
	}
	key := make([]byte, 24)
	_, _ = rand.Read(key)
	conf := fmt.Sprintf(settings, base64.RawURLEncoding.EncodeToString(key))
	// No published port: openbotd asks it through `docker exec`, so nothing else can reach it.
	// The settings go in through the environment and are written over the image's default.
	_, err := s.run(ctx, "run", "-d", "--name", container,
		"--label", "openbot.searxng="+label,
		"--restart", "unless-stopped",
		"--memory", "512m", "--cpus", "1", "--pids-limit", "256",
		"-e", "OPENBOT_SEARXNG_SETTINGS="+conf,
		"--entrypoint", "sh", Image,
		"-c", `printf '%s' "$OPENBOT_SEARXNG_SETTINGS" > /etc/searxng/settings.yml && exec /usr/local/searxng/entrypoint.sh`)
	return err
}

// Search runs a query.
func (s *SearXNG) Search(ctx context.Context, q Query) (Results, error) {
	if !s.Available() {
		return Results{}, ErrUnavailable
	}
	params := url.Values{"q": {q.Text}, "format": {"json"}}
	if q.Category != "" {
		params.Set("categories", q.Category)
	}
	if q.TimeRange != "" {
		params.Set("time_range", q.TimeRange)
	}
	if q.Language != "" {
		params.Set("language", q.Language)
	}
	if q.Page > 1 {
		params.Set("pageno", strconv.Itoa(q.Page))
	}
	var body []byte
	var err error
	if s.url != "" {
		body, err = s.get(ctx, s.url+"/search?"+params.Encode())
	} else {
		if err := s.ensure(ctx); err != nil {
			return Results{}, err
		}
		// One argument, no shell: the query can't break out of the URL.
		body, err = s.run(ctx, "exec", container, "wget", "-qO-", "-T", "20", "http://127.0.0.1:8080/search?"+params.Encode())
		if err != nil {
			err = errors.New("the search engine didn't answer")
		}
	}
	if err != nil {
		return Results{}, err
	}
	return parse(body)
}

func (s *SearXNG) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the search engine didn't answer: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, errors.New("the search engine refused: its JSON format isn't enabled (search.formats in settings.yml)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the search engine answered %s", resp.Status)
	}
	return body, nil
}

// MaxResults is how many hits a search returns.
const MaxResults = 10

const (
	maxTitle   = 200
	maxSnippet = 400
	maxURL     = 2000
)

func parse(body []byte) (Results, error) {
	var raw struct {
		Results []struct {
			Title         string `json:"title"`
			URL           string `json:"url"`
			Content       string `json:"content"`
			PublishedDate any    `json:"publishedDate"`
		} `json:"results"`
		Suggestions         []string   `json:"suggestions"`
		UnresponsiveEngines [][]string `json:"unresponsive_engines"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Results{}, errors.New("the search engine sent something that isn't a search result")
	}
	out := Results{Results: []Result{}}
	seen := map[string]bool{}
	for _, r := range raw.Results {
		u, ok := cleanURL(r.URL)
		if !ok || seen[u] {
			continue
		}
		seen[u] = true
		res := Result{Title: Clean(r.Title, maxTitle), URL: u, Snippet: Clean(r.Content, maxSnippet)}
		if d, ok := r.PublishedDate.(string); ok && len(d) >= 10 {
			res.Published = Clean(d[:10], 10) // the date is enough
		}
		out.Results = append(out.Results, res)
		if len(out.Results) == MaxResults {
			break
		}
	}
	for _, s := range raw.Suggestions {
		if s = Clean(s, 100); s != "" && len(out.Suggestions) < 5 {
			out.Suggestions = append(out.Suggestions, s)
		}
	}
	for _, e := range raw.UnresponsiveEngines {
		if len(e) > 0 {
			f := Clean(e[0], 40)
			if len(e) > 1 && e[1] != "" {
				f += " (" + Clean(e[1], 60) + ")"
			}
			out.Failed = append(out.Failed, f)
		}
	}
	return out, nil
}

// cleanURL keeps plain http(s) links only (no javascript:, data:, or credentials in the URL).
func cleanURL(raw string) (string, bool) {
	if len(raw) > maxURL {
		return "", false
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", false
	}
	return u.String(), true
}

// Clean makes text from the web safe to show a model: it drops control and format characters
// (zero-width spaces, bidi overrides, Unicode tag characters), which can hide instructions from a
// person reading along, collapses whitespace, and cuts it to max runes.
func Clean(s string, max int) string {
	var b strings.Builder
	space := false
	n := 0
	for _, r := range s {
		if r == utf8.RuneError {
			continue
		}
		if unicode.IsSpace(r) {
			space = b.Len() > 0
			continue
		}
		if unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Co, r) {
			continue
		}
		if n >= max {
			b.WriteRune('…')
			break
		}
		if space {
			b.WriteByte(' ')
			n++
			space = false
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}
