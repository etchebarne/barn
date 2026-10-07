package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Sign-in for MCP servers, following the MCP authorization spec: the server answers 401 and
// points at its protected-resource metadata (RFC 9728), which names an authorization server
// (RFC 8414). barn registers itself there (dynamic client registration, RFC 7591), sends the
// user to the authorize page with PKCE, and exchanges the code for tokens it refreshes.

// LoopbackRedirect is the redirect used when a server won't accept barn's own (plain http)
// address. Nothing listens there: the browser shows an error page and the user pastes its
// address back into barn.
const LoopbackRedirect = "http://127.0.0.1:8789/oauth/callback"

// ErrSignInRequired means an MCP server wants the user to sign in rather than a pasted key.
var ErrSignInRequired = errors.New("this server uses sign-in")

// OAuthTokens is what barn keeps (encrypted) for a signed-in connection.
type OAuthTokens struct {
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token,omitempty"`
	ExpiresAt     int64  `json:"expires_at,omitempty"` // unix seconds; 0 = unknown
	TokenEndpoint string `json:"token_endpoint"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret,omitempty"`
	Resource      string `json:"resource"`
}

// expiring reports whether the access token should be refreshed before use.
func (t OAuthTokens) expiring(now time.Time) bool {
	return t.ExpiresAt != 0 && now.Add(time.Minute).Unix() >= t.ExpiresAt
}

// oauthServer is what discovery learns about where and how to sign in.
type oauthServer struct {
	Resource              string
	Scope                 string
	AuthorizationEndpoint string
	TokenEndpoint         string
	RegistrationEndpoint  string
}

var resourceMetadataRe = regexp.MustCompile(`resource_metadata="([^"]+)"`)
var scopeRe = regexp.MustCompile(`scope="([^"]*)"`)

// discoverOAuth finds an MCP server's authorization server. It returns ErrNotOAuth (wrapped)
// when the server doesn't ask for sign-in.
func discoverOAuth(ctx context.Context, mcpURL string) (*oauthServer, error) {
	u, err := url.Parse(strings.TrimSpace(mcpURL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, userErr("the server URL must start with https:// or http://")
	}
	// Ask without credentials: a server that uses sign-in answers 401 with a pointer to its
	// metadata.
	s := &mcpSession{url: u.String()}
	resp, err := s.post(ctx, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": mcpProtocol, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "barn", "version": "1"},
	}})
	if err != nil {
		return nil, fmt.Errorf("reaching %s: %w", u.Host, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		return nil, userErr("%s doesn't ask for sign-in; connect it with a key instead (or without one)", u.Host)
	}
	challenge := resp.Header.Get("WWW-Authenticate")
	server := &oauthServer{Resource: canonicalResource(u)}
	if m := scopeRe.FindStringSubmatch(challenge); m != nil {
		server.Scope = m[1]
	}

	// Protected resource metadata: from the challenge, else the well-known locations.
	var prmURLs []string
	if m := resourceMetadataRe.FindStringSubmatch(challenge); m != nil {
		prmURLs = append(prmURLs, m[1])
	}
	origin := u.Scheme + "://" + u.Host
	if p := strings.TrimRight(u.Path, "/"); p != "" {
		prmURLs = append(prmURLs, origin+"/.well-known/oauth-protected-resource"+p)
	}
	prmURLs = append(prmURLs, origin+"/.well-known/oauth-protected-resource")
	var issuer string
	for _, prm := range prmURLs {
		var meta struct {
			Resource             string   `json:"resource"`
			AuthorizationServers []string `json:"authorization_servers"`
			ScopesSupported      []string `json:"scopes_supported"`
		}
		if getJSON(ctx, prm, &meta) == nil && len(meta.AuthorizationServers) > 0 {
			issuer = meta.AuthorizationServers[0]
			if meta.Resource != "" {
				server.Resource = meta.Resource
			}
			if server.Scope == "" && len(meta.ScopesSupported) > 0 {
				server.Scope = strings.Join(meta.ScopesSupported, " ")
			}
			break
		}
	}
	if issuer == "" {
		issuer = origin // older servers act as their own authorization server
	}

	// Authorization server metadata (RFC 8414, then OpenID Connect discovery).
	iu, err := url.Parse(strings.TrimRight(issuer, "/"))
	if err != nil {
		return nil, fmt.Errorf("bad authorization server %q", issuer)
	}
	ibase, ipath := iu.Scheme+"://"+iu.Host, iu.Path
	candidates := []string{ibase + "/.well-known/oauth-authorization-server" + ipath}
	if ipath != "" {
		candidates = append(candidates, ibase+"/.well-known/openid-configuration"+ipath, ibase+ipath+"/.well-known/openid-configuration")
	} else {
		candidates = append(candidates, ibase+"/.well-known/openid-configuration")
	}
	for _, c := range candidates {
		var meta struct {
			AuthorizationEndpoint string   `json:"authorization_endpoint"`
			TokenEndpoint         string   `json:"token_endpoint"`
			RegistrationEndpoint  string   `json:"registration_endpoint"`
			CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
		}
		if getJSON(ctx, c, &meta) != nil || meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
			continue
		}
		server.AuthorizationEndpoint, server.TokenEndpoint, server.RegistrationEndpoint =
			meta.AuthorizationEndpoint, meta.TokenEndpoint, meta.RegistrationEndpoint
		return server, nil
	}
	return nil, userErr("couldn't find where to sign in to %s", u.Host)
}

// canonicalResource is the MCP server's URL as the resource indicator (RFC 8707).
func canonicalResource(u *url.URL) string {
	c := *u
	c.Scheme, c.Host = strings.ToLower(c.Scheme), strings.ToLower(c.Host)
	c.RawQuery, c.Fragment = "", ""
	return strings.TrimRight(c.String(), "/")
}

func getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("MCP-Protocol-Version", mcpProtocol)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %d", u, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

// errRedirectRejected means the authorization server won't register this redirect URI.
var errRedirectRejected = errors.New("redirect URI rejected")

// registerClient registers barn with the authorization server (dynamic client registration).
func registerClient(ctx context.Context, server *oauthServer, redirectURI string) (clientID, clientSecret string, err error) {
	if server.RegistrationEndpoint == "" {
		return "", "", userErr("this server needs an app registered by hand, so barn can't sign in to it automatically yet")
	}
	body := map[string]any{
		"client_name":                "barn",
		"redirect_uris":              []string{redirectURI},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	}
	var out struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	status, err := postJSON(ctx, server.RegistrationEndpoint, body, &out)
	if err != nil {
		return "", "", err
	}
	if out.Error == "invalid_redirect_uri" || (status == http.StatusBadRequest && strings.Contains(strings.ToLower(out.Description), "redirect")) {
		return "", "", errRedirectRejected
	}
	if status/100 != 2 || out.ClientID == "" {
		return "", "", userErr("registering barn failed (%d): %s", status, firstNonEmpty(out.Description, out.Error))
	}
	return out.ClientID, out.ClientSecret, nil
}

func postJSON(ctx context.Context, u string, body, out any) (int, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(string(b)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
	return resp.StatusCode, nil
}

// OAuthFlow is a sign-in in progress, from the authorize redirect to the callback.
type OAuthFlow struct {
	State, Verifier string
	RedirectURI     string
	PasteBack       bool // the redirect goes to LoopbackRedirect; the user pastes the URL back
	MCPURL          string
	Server          oauthServer
	ClientID        string
	ClientSecret    string
	Created         time.Time

	// What to do once signed in.
	Name      string
	AgentIDs  []string
	MessageID string // a connect card to answer
	AccountID string // an existing connection to reconnect
}

// beginOAuth discovers the server, registers barn and builds the authorize URL. redirectURI
// is barn's own callback; when the server rejects it (plain http on a non-loopback address),
// the loopback redirect is used and the user pastes the result back.
func beginOAuth(ctx context.Context, mcpURL, redirectURI string) (*OAuthFlow, string, error) {
	server, err := discoverOAuth(ctx, mcpURL)
	if err != nil {
		return nil, "", err
	}
	flow := &OAuthFlow{State: randomToken(), Verifier: randomToken() + randomToken(), MCPURL: strings.TrimSpace(mcpURL),
		Server: *server, Created: time.Now(), RedirectURI: redirectURI}
	flow.ClientID, flow.ClientSecret, err = registerClient(ctx, server, redirectURI)
	if errors.Is(err, errRedirectRejected) && redirectURI != LoopbackRedirect {
		flow.RedirectURI, flow.PasteBack = LoopbackRedirect, true
		flow.ClientID, flow.ClientSecret, err = registerClient(ctx, server, LoopbackRedirect)
	}
	if errors.Is(err, errRedirectRejected) {
		return nil, "", userErr("%s won't accept barn's address for sign-in", hostOf(mcpURL))
	}
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256([]byte(flow.Verifier))
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {flow.ClientID},
		"redirect_uri":          {flow.RedirectURI},
		"state":                 {flow.State},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"resource":              {server.Resource},
	}
	if server.Scope != "" {
		q.Set("scope", server.Scope)
	}
	sep := "?"
	if strings.Contains(server.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	return flow, server.AuthorizationEndpoint + sep + q.Encode(), nil
}

// exchangeCode trades the authorization code for tokens.
func exchangeCode(ctx context.Context, flow *OAuthFlow, code string) (OAuthTokens, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {flow.RedirectURI},
		"code_verifier": {flow.Verifier},
		"resource":      {flow.Server.Resource},
	}
	t := OAuthTokens{TokenEndpoint: flow.Server.TokenEndpoint, ClientID: flow.ClientID, ClientSecret: flow.ClientSecret, Resource: flow.Server.Resource}
	return tokenRequest(ctx, t, form)
}

// refreshTokens gets a new access token with the refresh token.
func refreshTokens(ctx context.Context, t OAuthTokens) (OAuthTokens, error) {
	if t.RefreshToken == "" {
		return t, errReconnect
	}
	return tokenRequest(ctx, t, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.RefreshToken}, "resource": {t.Resource}})
}

// errReconnect means the user has to sign in again (the refresh token is gone or rejected).
var errReconnect = errors.New("sign-in expired")

func tokenRequest(ctx context.Context, t OAuthTokens, form url.Values) (OAuthTokens, error) {
	form.Set("client_id", t.ClientID)
	if t.ClientSecret != "" {
		form.Set("client_secret", t.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", t.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return t, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return t, err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken  string          `json:"access_token"`
		RefreshToken string          `json:"refresh_token"`
		ExpiresIn    json.Number     `json:"expires_in"`
		Error        string          `json:"error"`
		Description  string          `json:"error_description"`
		Raw          json.RawMessage `json:"-"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return t, fmt.Errorf("token endpoint: %d: %w", resp.StatusCode, err)
	}
	if out.Error == "invalid_grant" || out.Error == "invalid_client" || out.Error == "unauthorized_client" {
		return t, errReconnect
	}
	if resp.StatusCode/100 != 2 || out.AccessToken == "" {
		return t, userErr("signing in failed: %s", firstNonEmpty(out.Description, out.Error, resp.Status))
	}
	t.AccessToken = out.AccessToken
	if out.RefreshToken != "" {
		t.RefreshToken = out.RefreshToken // some servers rotate it
	}
	t.ExpiresAt = 0
	if secs, err := out.ExpiresIn.Int64(); err == nil && secs > 0 {
		t.ExpiresAt = time.Now().Add(time.Duration(secs) * time.Second).Unix()
	}
	return t, nil
}

// ParseCallback pulls the code and state out of a pasted callback URL.
func ParseCallback(raw string) (code, state string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", userErr("that doesn't look like the page's address")
	}
	q := u.Query()
	if e := q.Get("error"); e != "" {
		return "", "", userErr("sign-in was cancelled or refused (%s)", firstNonEmpty(q.Get("error_description"), e))
	}
	if q.Get("code") == "" || q.Get("state") == "" {
		return "", "", userErr("that address has no sign-in code; copy the whole address of the page you landed on")
	}
	return q.Get("code"), q.Get("state"), nil
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Hostname()
	}
	return raw
}

// IsLoopback reports whether a redirect origin is on this machine (http is fine there).
func IsLoopback(host string) bool {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
