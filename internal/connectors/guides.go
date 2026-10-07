package connectors

import (
	"encoding/json"
	"net/url"
)

// slackManifest describes the Slack app openbot needs: the bot scopes its tools use, Socket Mode
// (so no public URL is needed) and the events that wake agents.
var slackManifest = map[string]any{
	"display_information": map[string]any{
		"name":             "openbot",
		"description":      "Your openbot agents in Slack",
		"background_color": "#1d1d1f",
	},
	"features": map[string]any{
		"bot_user": map[string]any{"display_name": "openbot", "always_online": true},
		"app_home": map[string]any{"messages_tab_enabled": true, "messages_tab_read_only_enabled": false},
	},
	"oauth_config": map[string]any{
		"scopes": map[string]any{
			"bot": []string{
				"app_mentions:read", "chat:write", "channels:history", "channels:read",
				"groups:history", "groups:read", "im:history", "users:read",
			},
			// Posting as the user: messages show as them, with "Sent using openbot".
			"user": []string{"chat:write", "channels:read", "groups:read", "im:read", "mpim:read"},
		},
	},
	"settings": map[string]any{
		"event_subscriptions":    map[string]any{"bot_events": []string{"app_mention", "message.im"}},
		"socket_mode_enabled":    true,
		"org_deploy_enabled":     false,
		"token_rotation_enabled": false,
	},
}

func slackManifestJSON() string {
	b, _ := json.MarshalIndent(slackManifest, "", "  ")
	return string(b)
}

func (Slack) Setup() Setup {
	compact, _ := json.Marshal(slackManifest)
	return Setup{Steps: []SetupStep{
		{Text: "Create openbot's Slack app. The link opens Slack with everything filled in (permissions, " +
			"Socket Mode, events): pick your workspace, then click Next and Create.",
			Link: &SetupLink{Label: "Create Slack app", URL: "https://api.slack.com/apps?new_app=1&manifest_json=" + url.QueryEscape(string(compact))},
			Copy: &SetupCopy{Label: "Copy app manifest (if Slack shows an empty form, choose “From a manifest” and paste it)", Text: slackManifestJSON()}},
		{Text: "In the app's sidebar, open Install App, click Install to Workspace, then Allow."},
		{Text: "Copy the Bot User OAuth Token it shows (starts with xoxb-) into Bot token below. To let " +
			"agents post as you, also copy the User OAuth Token (starts with xoxp-) into User token."},
		{Text: "So agents hear mentions and DMs: open Basic Information, scroll to App-Level Tokens, click " +
			"Generate Token and Scopes, add the connections:write scope, click Generate, and paste the " +
			"token (starts with xapp-) into App-level token below."},
	}}
}

func (GitHub) Setup() Setup {
	q := url.Values{
		"name":          {"openbot"},
		"description":   {"Lets openbot agents read and manage issues and pull requests."},
		"issues":        {"write"},
		"pull_requests": {"write"},
		"metadata":      {"read"},
	}
	return Setup{
		Steps: []SetupStep{
			{Text: "Create a token on GitHub. The link fills in the name and permissions; under Repository " +
				"access choose the repositories agents may use, then click Generate token.",
				Link: &SetupLink{Label: "Create GitHub token", URL: "https://github.com/settings/personal-access-tokens/new?" + q.Encode()}},
			{Text: "Copy the token (starts with github_pat_) into Personal access token below."},
		},
		EventSteps: []SetupStep{
			{Text: "In the repository on GitHub, open Settings → Webhooks and click Add webhook."},
			{Text: "Paste the webhook URL and the secret shown here, set Content type to application/json, " +
				"choose the events you want (or Send me everything), and click Add webhook."},
		},
	}
}

func (GitHub) GeneratedSecret() string { return "webhook_secret" }

func (Linear) Setup() Setup {
	return Setup{
		Steps: []SetupStep{
			{Text: "Open Linear's security settings. Under Personal API keys, click New API key, name it openbot, " +
				"and create it.",
				Link: &SetupLink{Label: "Open Linear settings", URL: "https://linear.app/settings/account/security"}},
			{Text: "Copy the key (starts with lin_api_) into API key below."},
		},
		EventSteps: []SetupStep{
			{Text: "In Linear, open Settings → API → Webhooks and click New webhook.",
				Link: &SetupLink{Label: "Open Linear API settings", URL: "https://linear.app/settings/api"}},
			{Text: "Paste the webhook URL, choose Issues and Comments, and create the webhook."},
			{Text: "Copy its signing secret into Webhook signing secret here."},
		},
	}
}

func (Render) Setup() Setup {
	return Setup{
		Steps: []SetupStep{
			{Text: "Open your Render account settings and, under API Keys, click Create API Key and name it openbot.",
				Link: &SetupLink{Label: "Open Render settings", URL: "https://dashboard.render.com/u/settings#api-keys"}},
			{Text: "Copy the key (starts with rnd_) into API key below."},
		},
		EventSteps: []SetupStep{
			{Text: "In the Render dashboard, open Integrations → Webhooks and create a webhook with the URL shown here."},
			{Text: "Choose the events you want, then copy the webhook's signing secret (starts with whsec_) into " +
				"Webhook signing secret here."},
		},
	}
}

func (Webhook) Setup() Setup {
	return Setup{EventSteps: []SetupStep{
		{Text: "Have any service send a POST request with a JSON body to the URL shown here. Keep the URL " +
			"private: anyone who has it can send events to your agents."},
	}}
}

func (MCP) Setup() Setup {
	return Setup{Steps: []SetupStep{
		{Text: "Find the server's MCP URL in its documentation (it often ends in /mcp) and paste it into Server URL."},
		{Text: "If the server needs a key, paste it into Authorization header as “Bearer <key>”. Many servers " +
			"show the exact value in their setup instructions."},
	}}
}

func (MCPLocal) Setup() Setup {
	return Setup{Steps: []SetupStep{
		{Text: "Find the server's start command in its README; it usually looks like npx -y <package> or " +
			"uvx <package>. Paste it into Command."},
		{Text: "If the server needs keys, add them under Environment variables as NAME=value pairs separated " +
			"by spaces, using the names its README gives."},
	}}
}
