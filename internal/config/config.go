// Package config loads openbotd configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr          string
	DataDir       string
	WebDir        string
	SecureCookies bool
	// Extra origins allowed to open WebSocket connections (e.g. the Vite dev server).
	AllowedOrigins []string

	// Base URL of the OpenCode Go API. The API key itself is configured in the app.
	OpenCodeBaseURL string

	// CompactAtTokens is when agents' context gets compacted (0 = default).
	CompactAtTokens int

	// PublicURL is where external services reach openbotd (webhook URLs), e.g. a Tailscale Funnel
	// or reverse-proxy URL. Empty: the host the browser used.
	PublicURL string

	// Sandboxes: "docker" (default) or "off". SandboxImage overrides the default image.
	Sandboxes    string
	SandboxImage string
}

func Load() (Config, error) {
	c := Config{
		Addr:            env("OPENBOT_ADDR", "127.0.0.1:8080"),
		DataDir:         env("OPENBOT_DATA_DIR", "./data"),
		WebDir:          env("OPENBOT_WEB_DIR", ""),
		OpenCodeBaseURL: env("OPENBOT_OPENCODE_BASE_URL", "https://opencode.ai/zen/go/v1"),
		Sandboxes:       env("OPENBOT_SANDBOX", "docker"),
		PublicURL:       getenv("OPENBOT_PUBLIC_URL"),
		SandboxImage:    getenv("OPENBOT_SANDBOX_IMAGE"),
	}
	if c.Sandboxes != "docker" && c.Sandboxes != "off" {
		return c, fmt.Errorf("OPENBOT_SANDBOX must be docker or off")
	}

	var err error
	if c.SecureCookies, err = envBool("OPENBOT_SECURE_COOKIES", false); err != nil {
		return c, err
	}
	if v := getenv("OPENBOT_COMPACT_AT_TOKENS"); v != "" {
		if c.CompactAtTokens, err = strconv.Atoi(v); err != nil || c.CompactAtTokens < 1000 {
			return c, fmt.Errorf("OPENBOT_COMPACT_AT_TOKENS must be a number ≥ 1000")
		}
	}
	for o := range strings.SplitSeq(getenv("OPENBOT_ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}
	return c, nil
}

// getenv reads OPENBOT_<name>, falling back to the name from before the project was renamed
// (BARN_<name>) so existing configs keep working.
func getenv(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return os.Getenv("BARN_" + strings.TrimPrefix(key, "OPENBOT_"))
}

func env(key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) (bool, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return b, nil
}
