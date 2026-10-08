package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/openbot/internal/store"
)

// Secrets the user gives an agent: asked for with a secure field on a card (request_secret),
// never typed into chat. They're stored encrypted and reach the agent only as environment
// variables in run_command; anything a tool returns has their values masked, so the model never
// sees them.

const toolRequestSecret = "request_secret"

// maxSecretLength bounds a secret's value (keys, tokens, small JSON credentials).
const maxSecretLength = 64 << 10

var secretName = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)

// reservedSecretNames would break the shell if overridden.
var reservedSecretNames = []string{"PATH", "HOME", "USER", "SHELL", "PWD", "TERM", "LANG", "HOSTNAME", "LD_PRELOAD", "LD_LIBRARY_PATH"}

// ErrBadSecret is the user-facing reason a secret can't be saved.
type ErrBadSecret struct{ Message string }

func (e *ErrBadSecret) Error() string { return e.Message }

var requestSecretTool = function(toolRequestSecret,
	"Ask the user for a secret (API key, token, password) with a secure field on a card in your "+
		"DM, instead of having them paste it in chat. It's stored encrypted and you get it only as "+
		"an environment variable in run_command (e.g. $GITHUB_TOKEN); you never see the value, and "+
		"if a command prints it, it's masked. Your turn ends here; the outcome arrives as a "+
		"<secret_result>.",
	`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "Environment variable name, upper case, e.g. GITHUB_TOKEN or OPENAI_API_KEY."},
			"description": {"type": "string", "description": "What it is and where to get it, for the user, e.g. \"GitHub personal access token with repo scope (github.com/settings/tokens)\"."}
		},
		"required": ["name", "description"],
		"additionalProperties": false
	}`)

func checkSecretName(name string) error {
	if !secretName.MatchString(name) {
		return &ErrBadSecret{"a secret's name must look like an environment variable: upper case letters, digits and _, e.g. GITHUB_TOKEN"}
	}
	if slices.Contains(reservedSecretNames, name) {
		return &ErrBadSecret{name + " is used by the system; pick another name"}
	}
	return nil
}

// SetSecret saves (or replaces) one of an agent's secrets.
func (m *Manager) SetSecret(ctx context.Context, agentID, name, description, value string) error {
	if m.SecretBox == nil {
		return errors.New("secrets aren't available on this server")
	}
	if err := checkSecretName(name); err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return &ErrBadSecret{"the secret is empty"}
	}
	if len(value) > maxSecretLength {
		return &ErrBadSecret{"the secret is too long"}
	}
	if utf8.RuneCountInString(description) > 300 {
		return &ErrBadSecret{"the description is too long"}
	}
	sealed, err := m.SecretBox.Seal(value)
	if err != nil {
		return err
	}
	return m.store.SetAgentSecret(ctx, agentID, name, strings.TrimSpace(description), sealed)
}

// ProvideSecret answers a pending secret card with the value (saving it) and tells the agent.
// The value never goes into the message.
func (m *Manager) ProvideSecret(ctx context.Context, messageID, value string) (store.Message, error) {
	msg, err := m.store.GetMessage(ctx, messageID)
	if err != nil {
		return msg, err
	}
	p := msg.Prompt
	if p == nil || p.Kind != "secret" || p.Secret == nil || msg.AuthorAgentID == nil {
		return msg, store.ErrNotFound
	}
	if p.Status != "pending" {
		return msg, store.ErrPromptClosed
	}
	if err := m.SetSecret(ctx, *msg.AuthorAgentID, p.Secret.Name, p.Secret.Description, value); err != nil {
		return msg, err
	}
	msg, err = m.store.UpdatePrompt(ctx, messageID, func(p store.Prompt) (store.Prompt, error) {
		p.Status, p.Answer = "answered", &store.PromptAnswer{Selected: []int{0}}
		return p, nil
	})
	if err != nil {
		return msg, err
	}
	return msg, m.DeliverAnswer(ctx, msg)
}

func (l *loop) requestSecret(ctx context.Context, agent store.Agent, raw []byte) (string, bool) {
	var a struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	a.Name = strings.TrimSpace(a.Name)
	if err := checkSecretName(a.Name); err != nil {
		return toolError("%v", err), false
	}
	a.Description = strings.TrimSpace(a.Description)
	if a.Description == "" || utf8.RuneCountInString(a.Description) > 300 {
		return toolError("description must be 1–300 characters"), false
	}
	dm, err := l.m.store.DMChatID(ctx, agent.ID)
	if err != nil {
		return toolError("%v", err), false
	}
	msg, err := l.m.store.InsertPrompt(ctx, dm, agent.ID, store.Prompt{
		Kind:     "secret",
		Question: fmt.Sprintf("%s needs a secret: %s", agent.Name, a.Name),
		Options:  []store.PromptOption{{Label: "Save"}, {Label: "Not now"}},
		Secret:   &store.SecretRequest{Name: a.Name, Description: a.Description},
	})
	if err != nil {
		logger(agent.ID).Error("request_secret", "err", err)
		return toolError("failed to ask for the secret"), false
	}
	l.m.publishMessage(msg)
	return toolOK(map[string]string{
		"prompt_id": msg.ID,
		"note":      "Asked with a secure field in your DM. Your turn ends here; the outcome arrives as a <secret_result>.",
	}), true
}

func renderSecretAnswer(msg store.Message) string {
	p := msg.Prompt
	if len(p.Answer.Selected) > 0 && p.Answer.Selected[0] == 0 {
		return fmt.Sprintf("<secret_result prompt_id=%q name=%q provided=\"true\">\nSaved. Use it as $%s in "+
			"run_command; never print it or put it in messages or files others can read.\n</secret_result>",
			msg.ID, p.Secret.Name, p.Secret.Name)
	}
	return fmt.Sprintf("<secret_result prompt_id=%q name=%q provided=\"false\">\nThe user didn't provide it.\n</secret_result>",
		msg.ID, p.Secret.Name)
}

// secretValues returns the agent's secrets, decrypted, by name.
func (m *Manager) secretValues(ctx context.Context, agentID string) (map[string]string, error) {
	if m.SecretBox == nil {
		return nil, nil
	}
	sealed, err := m.store.SealedAgentSecrets(ctx, agentID)
	if err != nil || len(sealed) == 0 {
		return nil, err
	}
	out := make(map[string]string, len(sealed))
	for name, s := range sealed {
		v, err := m.SecretBox.Open(s)
		if err != nil {
			return nil, fmt.Errorf("secret %s: %w", name, err)
		}
		out[name] = v
	}
	return out, nil
}

// secretEnv is the environment run_command gets: the agent's secrets.
func (l *loop) secretEnv(ctx context.Context, agent store.Agent) (map[string]string, error) {
	return l.m.secretValues(ctx, agent.ID)
}

// maskSecrets replaces the agent's secret values in a tool result with [secret NAME], both as
// they are and as they'd appear inside JSON strings.
func (l *loop) maskSecrets(ctx context.Context, agentID, text string) string {
	values, err := l.m.secretValues(ctx, agentID)
	if err != nil || len(values) == 0 {
		return text
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	// Longest first, so a secret containing another is masked whole.
	slices.SortFunc(names, func(a, b string) int { return len(values[b]) - len(values[a]) })
	for _, name := range names {
		v := values[name]
		if len(v) < 4 {
			continue // too short to mask without mangling ordinary text
		}
		mask := "[secret " + name + "]"
		text = strings.ReplaceAll(text, v, mask)
		if b, err := json.Marshal(v); err == nil {
			if escaped := string(b[1 : len(b)-1]); escaped != v {
				text = strings.ReplaceAll(text, escaped, mask)
			}
		}
	}
	return text
}

// writeSecrets lists the agent's secrets (names only) in its system prompt.
func (l *loop) writeSecrets(ctx context.Context, b *strings.Builder, agent store.Agent) {
	secrets, err := l.m.store.AgentSecrets(ctx, agent.ID)
	if err != nil || len(secrets) == 0 {
		return
	}
	b.WriteString("- Your secrets, as environment variables in run_command (you never see the values; don't print them):")
	for _, s := range secrets {
		b.WriteString(" $" + s.Name)
		if s.Description != "" {
			b.WriteString(" (" + s.Description + ")")
		}
		b.WriteString(";")
	}
	b.WriteString("\n")
}
