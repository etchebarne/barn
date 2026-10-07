package connectors

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

var httpClient = &http.Client{Timeout: 60 * time.Second}

// apiRequest sends a JSON request and decodes a JSON response. Non-2xx responses become errors
// that include the service's message.
func apiRequest(ctx context.Context, method, url string, header http.Header, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "barn")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		msg := strings.TrimSpace(string(raw))
		var e struct {
			Message string `json:"message"`
			Error   any    `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Message != "" {
			msg = e.Message
		}
		if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return userErr("the service rejected the credentials (%d): %s", resp.StatusCode, msg)
		}
		if resp.StatusCode == http.StatusNotFound {
			return userErr("not found (404): %s", msg)
		}
		return fmt.Errorf("%s %s: %d: %s", method, url, resp.StatusCode, msg)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// validHMACSHA256 checks a hex HMAC-SHA256 signature of body.
func validHMACSHA256(secret string, body []byte, signatureHex string) bool {
	if secret == "" || signatureHex == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := mac.Sum(nil)
	got, err := hex.DecodeString(signatureHex)
	return err == nil && hmac.Equal(got, want)
}

// params is a JSON schema for an object with the given properties ("name": "description",
// optional ones prefixed with "?").
func params(props map[string]string) json.RawMessage {
	properties := map[string]any{}
	var required []string
	for name, desc := range props {
		typ := "string"
		if strings.HasPrefix(desc, "int:") {
			typ, desc = "integer", strings.TrimPrefix(desc, "int:")
		}
		if strings.HasPrefix(name, "?") {
			name = strings.TrimPrefix(name, "?")
		} else {
			required = append(required, name)
		}
		properties[name] = map[string]string{"type": typ, "description": desc}
	}
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		slices.Sort(required)
		schema["required"] = required
	}
	b, _ := json.Marshal(schema)
	return b
}

// decodeArgs unmarshals tool arguments, reporting problems as user errors.
func decodeArgs(args json.RawMessage, v any) error {
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(args, v); err != nil {
		return userErr("invalid arguments: %v", err)
	}
	return nil
}

// str reads a nested string field from decoded JSON ("a.b.c").
func str(m map[string]any, path string) string {
	var cur any = m
	for _, key := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = obj[key]
	}
	switch v := cur.(type) {
	case string:
		return v
	case float64:
		return strings.TrimSuffix(fmt.Sprintf("%f", v), ".000000")
	case bool:
		return fmt.Sprint(v)
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func baseURL(acct Account, def string) string {
	if u := strings.TrimRight(acct.Config["base_url"], "/"); u != "" {
		return u
	}
	return def
}

func randomToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
