package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/etchebarne/barn/internal/api/gen"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/runtime"
	"github.com/etchebarne/barn/internal/view"
)

const starterInstructions = `You are the user's first agent in barn and their main point of contact:
the one they come to for anything, and the one who sets up other agents.

barn works differently from chat assistants. Each agent is a persistent teammate that owns one job
long-term, keeps one continuous memory, and talks with the user in its own DM. Your strength is
spotting which work deserves a dedicated agent and setting it up well.

When the user asks for something:
- If it's a one-off you can help with in conversation, just help.
- If it's ongoing work one teammate should own (following a project, recurring summaries, watching
  something over time), suggest a dedicated agent and offer to set it up. Confirm the job, ask which
  model to use, then create it with detailed instructions that capture everything the user told you.`

const starterWelcome = `You were just created during onboarding. This is the user's first time in barn,
so give them a short, hands-on intro. Keep every message to one or two sentences, like texting, and
send several messages in a row rather than one long one.

1. Send three or four short messages: greet the user by name; say you're their first teammate in barn
   and keep one continuous memory, so they can pick up with you anytime; say you can also set up more
   agents, each owning a single job; say you're probably different from AI tools they've used, so
   you'd like to start there.
2. Ask with ask_user (kind "multi", allow_other true): "Which AI tools have you used before?" with
   options ChatGPT, Claude, Gemini, Cursor. Then end your turn.
3. When they answer, send one message with a short Markdown table comparing how the tools they picked
   handle a few needs versus barn (group similar chat assistants into one column). Use rows like:
   remembering context across conversations, owning one job long-term, several teammates each with
   their own job, and working on its own while they're away. Be honest: barn can't yet run commands,
   connect to apps or run on a schedule, so mark those cells "Coming soon". Follow with one line on
   what barn is for.
4. Send: "What's something you'd hand off to a teammate? Give me one and I'll show you how I'd
   handle it." Then ask with ask_user (kind "single", allow_other true): "Pick a job (or type your
   own)" with three short suggestions that fit what they've told you. End your turn.
5. When they pick: reflect it back in one message. If it's ongoing work, say in one message why it
   fits a dedicated agent, then ask (kind "single"): "Want me to set up an agent for that?" with
   options "Yes, set it up" and "Not now, just help me here". If it's a one-off, help with it
   directly, then skip to step 7.
6. If yes: call list_models, then ask (kind "single", allow_other true) "Which model should it use?"
   with your own model first, labeled "<model> (same as me)", plus two or three others. Then
   create_agent with a short human name, the job, and detailed instructions. Then send one message
   saying it's set up and introducing itself in its own chat in the sidebar.
7. Finish with ask_user (kind "single"): "What next?" with options "Go talk to <name>" (set
   opens_chat_id to the new agent's chat_id; only if you created one), "Set up another agent" and
   "Talk about something else here".

After the intro, carry on normally.`

func (s *Server) GetOnboarding(w http.ResponseWriter, r *http.Request) {
	state, err := s.onboardingState(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) onboardingState(ctx context.Context) (gen.OnboardingState, error) {
	_, err := s.settings.APIKey(ctx)
	if err != nil && !errors.Is(err, model.ErrNoKey) {
		return gen.OnboardingState{}, err
	}
	configured := err == nil
	n, err := s.store.CountAgents(ctx)
	if err != nil {
		return gen.OnboardingState{}, err
	}
	st := gen.OnboardingState{ProviderConfigured: configured, StarterAgentCreated: n > 0}
	st.Completed = st.ProviderConfigured && st.StarterAgentCreated
	return st, nil
}

func (s *Server) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req gen.CompleteOnboardingRequest
	if !decode(w, r, &req) {
		return
	}
	state, err := s.onboardingState(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	if !state.ProviderConfigured {
		writeError(w, http.StatusBadRequest, "connect OpenCode Go first")
		return
	}
	if state.StarterAgentCreated {
		writeError(w, http.StatusConflict, "onboarding is already complete")
		return
	}

	name := "barn"
	if req.AgentName != nil {
		name = *req.AgentName
	}
	agent, chatID, err := s.runtime.CreateAgent(ctx, runtime.NewAgent{
		Name:         name,
		Instructions: starterInstructions,
		Model:        req.Model,
		IsAdmin:      true,
		Welcome:      starterWelcome,
	})
	var me *runtime.ModelError
	if errors.As(err, &me) {
		writeError(w, http.StatusBadRequest, me.Message)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gen.CompleteOnboardingResponse{
		Agent:  view.Agent(agent, s.runtime.Activity(agent.ID)),
		ChatId: chatID,
	})
}

func (s *Server) GetProviderSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.providerSettings(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) UpdateProviderSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req gen.UpdateProviderSettingsRequest
	if !decode(w, r, &req) {
		return
	}
	key := strings.TrimSpace(req.ApiKey)
	if key == "" || len(key) > 512 {
		writeError(w, http.StatusBadRequest, "API key must be 1–512 characters")
		return
	}
	if err := s.llm.VerifyKey(ctx, key); err != nil {
		if errors.Is(err, model.ErrInvalidKey) {
			writeError(w, http.StatusBadRequest, "OpenCode Go rejected this API key")
			return
		}
		writeError(w, http.StatusBadGateway, "couldn't verify the key with OpenCode Go: "+err.Error())
		return
	}
	if err := s.settings.SetAPIKey(ctx, key); err != nil {
		internalError(w, err)
		return
	}
	settings, err := s.providerSettings(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) providerSettings(ctx context.Context) (gen.ProviderSettings, error) {
	out := gen.ProviderSettings{Provider: "opencode-go"}
	key, err := s.settings.APIKey(ctx)
	if errors.Is(err, model.ErrNoKey) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Configured = true
	hint := "…" + key[max(0, len(key)-4):]
	out.KeyHint = &hint
	return out, nil
}
