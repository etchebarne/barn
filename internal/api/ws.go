package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
)

// serveWS streams bus events to an authenticated client. The connection is send-only; clients
// use the REST API for actions.
func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: originHosts(s.opts.AllowedOrigins),
	})
	if err != nil {
		slog.Debug("ws accept", "err", err)
		return
	}
	defer conn.CloseNow()

	sub := s.bus.Subscribe()
	defer sub.Close()

	// Reading is required to process control frames (ping/pong/close).
	ctx := conn.CloseRead(r.Context())
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-sub.C:
			if !ok {
				conn.Close(websocket.StatusTryAgainLater, "fell behind; reconnect to resync")
				return
			}
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Write(wctx, websocket.MessageText, data)
			cancel()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// originHosts converts allowed origins (full URLs) into the host patterns websocket expects.
// Same-origin requests are always allowed.
func originHosts(origins []string) []string {
	hosts := make([]string, 0, len(origins))
	for _, o := range origins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			hosts = append(hosts, u.Host)
		}
	}
	return hosts
}
