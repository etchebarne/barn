package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/etchebarne/barn/internal/api/gen"
	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
	"github.com/etchebarne/barn/internal/view"
)

const starterInstructions = `You are the user's first agent in barn and their main point of contact.
Help with whatever they ask. Soon you'll be able to create and manage other agents, scheduled tasks,
memories, and connectors (Slack, Linear, and more) on their behalf. Those abilities aren't available
yet; if asked, say they're coming.`

const starterWelcome = "You were just created during onboarding. Send a short hello in your DM: " +
	"introduce yourself in one or two sentences and ask what they'd like help with."

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
		name = strings.TrimSpace(*req.AgentName)
	}
	if name == "" || utf8.RuneCountInString(name) > 64 {
		writeError(w, http.StatusBadRequest, "agent name must be 1–64 characters")
		return
	}
	models, err := s.llm.Models(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "couldn't load models: "+err.Error())
		return
	}
	if !slices.Contains(models, req.Model) {
		writeError(w, http.StatusBadRequest, "unknown model "+req.Model)
		return
	}

	agent, chatID, err := s.store.CreateAgentWithDM(ctx, store.Agent{
		Name:          name,
		Instructions:  starterInstructions,
		Model:         req.Model,
		Language:      "auto",
		Notifications: true,
		TrustMode:     "ask",
		IsAdmin:       true,
	})
	if err != nil {
		internalError(w, err)
		return
	}
	s.runtime.AddAgent(agent.ID)
	if err := s.runtime.Notify(ctx, agent.ID, starterWelcome); err != nil {
		internalError(w, err)
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
