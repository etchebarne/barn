// Package push sends Web Push notifications to the user's subscribed browsers.
package push

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/etchebarne/barn/internal/secrets"
	"github.com/etchebarne/barn/internal/store"
)

type Notifier struct {
	store   *store.Store
	public  string
	private string
	// Subject identifies the sender to push services (a mailto: or https: URL).
	Subject string
	// Client sends the push requests (tests swap it).
	Client *http.Client
}

// New loads the VAPID key pair, generating and storing it on first use.
func New(ctx context.Context, st *store.Store, box *secrets.Box) (*Notifier, error) {
	n := &Notifier{store: st, Subject: "https://github.com/etchebarne/barn", Client: http.DefaultClient}
	public, err := st.GetSetting(ctx, store.SettingVAPIDPublic)
	if errors.Is(err, store.ErrNotFound) {
		private, pub, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			return nil, err
		}
		sealed, err := box.Seal(private)
		if err != nil {
			return nil, err
		}
		if err := st.SetSetting(ctx, store.SettingVAPIDPrivate, sealed); err != nil {
			return nil, err
		}
		if err := st.SetSetting(ctx, store.SettingVAPIDPublic, pub); err != nil {
			return nil, err
		}
		n.public, n.private = pub, private
		return n, nil
	}
	if err != nil {
		return nil, err
	}
	sealed, err := st.GetSetting(ctx, store.SettingVAPIDPrivate)
	if err != nil {
		return nil, err
	}
	if n.private, err = box.Open(sealed); err != nil {
		return nil, err
	}
	n.public = public
	return n, nil
}

// PublicKey is the VAPID application server key browsers subscribe with.
func (n *Notifier) PublicKey() string { return n.public }

// Notification is what the service worker shows.
type Notification struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	ChatID string `json:"chatId"`
	// Tag groups notifications: a newer one for the same chat replaces the older.
	Tag string `json:"tag"`
}

// Send delivers a notification to every subscribed browser, dropping subscriptions the push
// service says are gone.
func (n *Notifier) Send(ctx context.Context, note Notification) {
	subs, err := n.store.PushSubscriptions(ctx)
	if err != nil {
		slog.Error("push: load subscriptions", "err", err)
		return
	}
	payload, _ := json.Marshal(note)
	for _, sub := range subs {
		resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
		}, &webpush.Options{
			HTTPClient:      n.Client,
			Subscriber:      n.Subject,
			VAPIDPublicKey:  n.public,
			VAPIDPrivateKey: n.private,
			TTL:             60 * 60 * 24,
			Urgency:         webpush.UrgencyHigh,
		})
		if err != nil {
			slog.Warn("push: send", "err", err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound:
			_ = n.store.DeletePushSubscription(ctx, sub.Endpoint)
		case resp.StatusCode >= 400:
			slog.Warn("push: rejected", "status", resp.StatusCode)
		}
	}
}

// Preview turns a Markdown message into a short plain-text notification body.
func Preview(markdown string) string {
	r := strings.NewReplacer("```", "", "**", "", "__", "", "`", "", "#", "", "> ", "", "|", " ")
	text := strings.Join(strings.Fields(r.Replace(markdown)), " ")
	if utf8.RuneCountInString(text) > 160 {
		text = string([]rune(text)[:157]) + "…"
	}
	return text
}
