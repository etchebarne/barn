// Package view converts store records into API representations.
package view

import (
	"github.com/etchebarne/barn/internal/api/gen"
	"github.com/etchebarne/barn/internal/store"
)

func Message(m store.Message) gen.Message {
	return gen.Message{
		Id:        m.ID,
		ChatId:    m.ChatID,
		Author:    gen.MessageAuthor{Kind: gen.MessageAuthorKind(m.AuthorKind), AgentId: m.AuthorAgentID},
		Body:      m.Body,
		CreatedAt: store.Time(m.CreatedAt),
		Failure:   failure(m.Failure),
		Prompt:    prompt(m.Prompt),
		Event:     event(m.Event),
	}
}

func prompt(p *store.Prompt) *gen.Prompt {
	if p == nil {
		return nil
	}
	options := make([]gen.PromptOption, 0, len(p.Options))
	for _, o := range p.Options {
		opt := gen.PromptOption{Label: o.Label}
		if o.OpensChatID != "" {
			opt.OpensChatId = &o.OpensChatID
		}
		options = append(options, opt)
	}
	out := &gen.Prompt{
		Kind:       gen.PromptKind(p.Kind),
		Question:   p.Question,
		Options:    options,
		AllowOther: p.AllowOther,
		Status:     gen.PromptStatus(p.Status),
	}
	if p.Answer != nil {
		a := gen.PromptAnswer{}
		if len(p.Answer.Selected) > 0 {
			sel := p.Answer.Selected
			a.Selected = &sel
		}
		if p.Answer.Text != "" {
			text := p.Answer.Text
			a.Text = &text
		}
		out.Answer = &a
	}
	return out
}

func event(e *store.MessageEvent) *gen.MessageEvent {
	if e == nil {
		return nil
	}
	out := &gen.MessageEvent{Kind: gen.MessageEventKind(e.Kind), AgentId: e.AgentID}
	if e.ChatID != "" {
		out.ChatId = &e.ChatID
	}
	return out
}

func MessageUpdated(m gen.Message) gen.WsMessageUpdated {
	return gen.WsMessageUpdated{Type: "message.updated", Message: m}
}

func failure(f *store.Failure) *gen.MessageFailure {
	if f == nil {
		return nil
	}
	return &gen.MessageFailure{
		AgentId:   f.AgentID,
		Reason:    gen.MessageFailureReason(f.Reason),
		Retryable: f.Retryable,
	}
}

func Agent(a store.Agent, activity gen.AgentActivity) gen.Agent {
	return gen.Agent{
		Id:            a.ID,
		Name:          a.Name,
		Model:         a.Model,
		Language:      a.Language,
		Notifications: a.Notifications,
		TrustMode:     gen.AgentTrustMode(a.TrustMode),
		IsAdmin:       a.IsAdmin,
		Activity:      activity,
		CreatedAt:     store.Time(a.CreatedAt),
	}
}

func Chat(c store.Chat) gen.Chat {
	members := make([]gen.ChatMember, 0, len(c.Members))
	for _, m := range c.Members {
		members = append(members, gen.ChatMember{AgentId: m.AgentID, Position: m.Position})
	}
	out := gen.Chat{
		Id:          c.ID,
		Kind:        gen.ChatKind(c.Kind),
		Name:        c.Name,
		Members:     members,
		UnreadCount: c.UnreadCount,
		CreatedAt:   store.Time(c.CreatedAt),
	}
	if c.LastMessage != nil {
		m := Message(*c.LastMessage)
		out.LastMessage = &m
	}
	return out
}

func MessageCreated(m gen.Message) gen.WsMessageCreated {
	return gen.WsMessageCreated{Type: "message.created", Message: m}
}

func Idle() gen.AgentActivity { return gen.AgentActivity{State: "idle"} }

func Working(label string) gen.AgentActivity {
	return gen.AgentActivity{State: "working", Label: &label}
}
