package connectors

import (
	"context"
	"time"
)

// CatalogApp is an app that connects by signing in to its official MCP server.
type CatalogApp struct {
	ID, Name, Description, URL string
}

// Catalog lists servers checked to support sign-in with automatic app registration. (Others
// that need an app registered by hand, like GitHub's and Asana's, aren't here yet.)
var Catalog = []CatalogApp{
	{ID: "linear", Name: "Linear", Description: "Find, create and update issues, projects and comments.", URL: "https://mcp.linear.app/mcp"},
	{ID: "notion", Name: "Notion", Description: "Search, read and edit pages and databases.", URL: "https://mcp.notion.com/mcp"},
	{ID: "sentry", Name: "Sentry", Description: "Look into errors, issues and releases.", URL: "https://mcp.sentry.dev/mcp"},
	{ID: "stripe", Name: "Stripe", Description: "Look up customers, payments and subscriptions.", URL: "https://mcp.stripe.com"},
}

// UsesSignIn reports whether an MCP server asks for an OAuth sign-in.
func UsesSignIn(ctx context.Context, mcpURL string) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := discoverOAuth(ctx, mcpURL)
	return err == nil
}

// PeekOAuth returns a pending sign-in without using it up (to know where to send the user
// back when the service reports an error).
func PeekOAuth(state string) (*OAuthFlow, bool) {
	oauth.mu.Lock()
	defer oauth.mu.Unlock()
	f, ok := oauth.flows[state]
	return f, ok
}
