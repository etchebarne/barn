// Package config loads barnd configuration from environment variables.
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

	// Sandboxes: "docker" (default) or "off". SandboxImage overrides the default image.
	Sandboxes    string
	SandboxImage string
}

func Load() (Config, error) {
	c := Config{
		Addr:            env("BARN_ADDR", "127.0.0.1:8080"),
		DataDir:         env("BARN_DATA_DIR", "./data"),
		WebDir:          env("BARN_WEB_DIR", ""),
		OpenCodeBaseURL: env("BARN_OPENCODE_BASE_URL", "https://opencode.ai/zen/go/v1"),
		Sandboxes:       env("BARN_SANDBOX", "docker"),
		SandboxImage:    os.Getenv("BARN_SANDBOX_IMAGE"),
	}
	if c.Sandboxes != "docker" && c.Sandboxes != "off" {
		return c, fmt.Errorf("BARN_SANDBOX must be docker or off")
	}

	var err error
	if c.SecureCookies, err = envBool("BARN_SECURE_COOKIES", false); err != nil {
		return c, err
	}
	if v := os.Getenv("BARN_COMPACT_AT_TOKENS"); v != "" {
		if c.CompactAtTokens, err = strconv.Atoi(v); err != nil || c.CompactAtTokens < 1000 {
			return c, fmt.Errorf("BARN_COMPACT_AT_TOKENS must be a number ≥ 1000")
		}
	}
	for o := range strings.SplitSeq(os.Getenv("BARN_ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}
	return c, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return b, nil
}
