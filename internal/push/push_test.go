package push

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/hkdf"

	"github.com/etchebarne/openbot/internal/secrets"
	"github.com/etchebarne/openbot/internal/store"
)

// browser simulates a subscribed browser's keys and decrypts what the push service receives
// (RFC 8291 / RFC 8188, aes128gcm).
type browser struct {
	priv *ecdh.PrivateKey
	auth []byte
}

func newBrowser(t *testing.T) browser {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	rand.Read(auth)
	return browser{priv: priv, auth: auth}
}

func (b browser) keys() (p256dh, auth string) {
	enc := base64.RawURLEncoding
	return enc.EncodeToString(b.priv.PublicKey().Bytes()), enc.EncodeToString(b.auth)
}

func (b browser) decrypt(t *testing.T, body []byte) []byte {
	t.Helper()
	salt, rs, idlen := body[:16], binary.BigEndian.Uint32(body[16:20]), int(body[20])
	_ = rs
	asPublicBytes := body[21 : 21+idlen]
	ciphertext := body[21+idlen:]
	asPublic, err := ecdh.P256().NewPublicKey(asPublicBytes)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := b.priv.ECDH(asPublic)
	if err != nil {
		t.Fatal(err)
	}
	info := append(append([]byte("WebPush: info\x00"), b.priv.PublicKey().Bytes()...), asPublicBytes...)
	ikm := make([]byte, 32)
	io.ReadFull(hkdf.New(sha256.New, secret, b.auth, info), ikm)
	read := func(label string, n int) []byte {
		out := make([]byte, n)
		io.ReadFull(hkdf.New(sha256.New, ikm, salt, []byte(label)), out)
		return out
	}
	cek, nonce := read("Content-Encoding: aes128gcm\x00", 16), read("Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	// Strip the padding delimiter (0x02) and any zero padding.
	plain = bytes.TrimRight(plain, "\x00")
	return plain[:len(plain)-1]
}

func TestSendDeliversEncryptedNotifications(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	box, _ := secrets.Open(dir)
	n, err := New(ctx, st, box)
	if err != nil {
		t.Fatal(err)
	}
	// Keys persist across restarts.
	if again, _ := New(ctx, st, box); again.PublicKey() != n.PublicKey() {
		t.Fatal("VAPID keys should be stable")
	}

	var mu sync.Mutex
	var bodies [][]byte
	var authHeaders []string
	service := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, b)
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/gone") {
			w.WriteHeader(http.StatusGone)
			return
		}
		if r.Header.Get("Content-Encoding") != "aes128gcm" {
			t.Errorf("Content-Encoding = %q", r.Header.Get("Content-Encoding"))
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer service.Close()
	n.Client = service.Client()

	phone := newBrowser(t)
	p256dh, auth := phone.keys()
	st.SavePushSubscription(ctx, store.PushSubscription{Endpoint: service.URL + "/phone", P256dh: p256dh, Auth: auth})
	old := newBrowser(t)
	p2, a2 := old.keys()
	st.SavePushSubscription(ctx, store.PushSubscription{Endpoint: service.URL + "/gone", P256dh: p2, Auth: a2})

	n.Send(ctx, Notification{Title: "Tracker", Body: "Taxes are unblocked", ChatID: "chat-1", Tag: "chat-1"})

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("expected 2 deliveries, got %d", len(bodies))
	}
	var got Notification
	if err := json.Unmarshal(phone.decrypt(t, bodies[0]), &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != "Tracker" || got.Body != "Taxes are unblocked" || got.ChatID != "chat-1" {
		t.Fatalf("decrypted notification = %+v", got)
	}
	if !strings.HasPrefix(authHeaders[0], "vapid t=") || !strings.Contains(authHeaders[0], "k="+n.PublicKey()) {
		t.Fatalf("missing VAPID auth: %q", authHeaders[0])
	}
	subs, _ := st.PushSubscriptions(ctx)
	if len(subs) != 1 || !strings.HasSuffix(subs[0].Endpoint, "/phone") {
		t.Fatalf("the gone subscription should be removed, have %+v", subs)
	}
}

func TestPreview(t *testing.T) {
	if got := Preview("## Your week\n\n- **openbot**: shipped `v0.2`"); got != "Your week - openbot: shipped v0.2" {
		t.Fatalf("preview = %q", got)
	}
	if got := Preview(strings.Repeat("word ", 100)); len([]rune(got)) != 158 || !strings.HasSuffix(got, "…") {
		t.Fatalf("long previews are clipped: %d %q", len([]rune(got)), got[len(got)-10:])
	}
}
