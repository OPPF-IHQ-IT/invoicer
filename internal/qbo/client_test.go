package qbo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/storage"
)

// fakeIntuit serves both the token endpoint and the QBO API.
type fakeIntuit struct {
	validToken string // the access token the API accepts
	refreshes  int
	apiCalls   int
	bodies     []string // request bodies the API received
	refreshErr bool
}

func (f *fakeIntuit) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			f.refreshes++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parsing token form: %v", err)
			}
			if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
				t.Errorf("grant_type = %q, want refresh_token", got)
			}
			if f.refreshErr {
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			f.validToken = "fresh-access"
			io.WriteString(w, `{"access_token":"fresh-access","refresh_token":"rotated-refresh","token_type":"bearer","expires_in":3600}`)
			return
		}
		f.apiCalls++
		body, _ := io.ReadAll(r.Body)
		f.bodies = append(f.bodies, string(body))
		if r.Header.Get("Authorization") != "Bearer "+f.validToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		io.WriteString(w, `{"ok":true}`)
	})
}

func newTestClient(t *testing.T, f *fakeIntuit, tok *storage.Token) *Client {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // SaveToken writes under $HOME/.config/invoicer
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)

	oldTokenURL := tokenURL
	tokenURL = srv.URL + "/token"
	t.Cleanup(func() { tokenURL = oldTokenURL })

	cfg := &config.Config{}
	cfg.QBO.Environment = "sandbox"
	cfg.QBO.SandboxBaseURL = srv.URL
	cfg.QBO.ClientID = "id"
	cfg.QBO.ClientSecret = "secret"
	return &Client{cfg: cfg, token: tok, http: srv.Client(), realmID: "123"}
}

func TestExpiredTokenIsRefreshedBeforeRequest(t *testing.T) {
	f := &fakeIntuit{validToken: "fresh-access"}
	c := newTestClient(t, f, &storage.Token{
		AccessToken: "stale-access", RefreshToken: "old-refresh",
		ExpiresAt: time.Now().Add(-time.Minute).Unix(), RealmID: "123",
	})

	if _, err := c.query(context.Background(), "SELECT * FROM Item"); err != nil {
		t.Fatalf("query: %v", err)
	}
	if f.refreshes != 1 || f.apiCalls != 1 {
		t.Errorf("refreshes=%d apiCalls=%d, want 1 and 1", f.refreshes, f.apiCalls)
	}

	saved, err := storage.LoadToken(c.cfg)
	if err != nil {
		t.Fatalf("loading saved token: %v", err)
	}
	if saved.AccessToken != "fresh-access" || saved.RefreshToken != "rotated-refresh" || saved.RealmID != "123" {
		t.Errorf("saved token = %+v, want refreshed access/refresh tokens and the same realm", saved)
	}
}

func TestUnauthorizedRetriesOnceWithSameBody(t *testing.T) {
	// The token looks valid locally but the API rejects it (e.g. revoked).
	f := &fakeIntuit{validToken: "something-else"}
	c := newTestClient(t, f, &storage.Token{
		AccessToken: "looks-valid", RefreshToken: "old-refresh",
		ExpiresAt: time.Now().Add(time.Hour).Unix(), RealmID: "123",
	})

	if _, err := c.post(context.Background(), "invoice", map[string]string{"DocNumber": "1001"}); err != nil {
		t.Fatalf("post: %v", err)
	}
	if f.refreshes != 1 || f.apiCalls != 2 {
		t.Errorf("refreshes=%d apiCalls=%d, want 1 and 2", f.refreshes, f.apiCalls)
	}
	if len(f.bodies) != 2 || f.bodies[0] != f.bodies[1] || !strings.Contains(f.bodies[1], "1001") {
		t.Errorf("retry body = %q, want the original body resent", f.bodies)
	}
}

func TestRefreshFailureAsksForLogin(t *testing.T) {
	f := &fakeIntuit{refreshErr: true}
	c := newTestClient(t, f, &storage.Token{
		AccessToken: "stale-access", RefreshToken: "expired-refresh",
		ExpiresAt: time.Now().Add(-time.Minute).Unix(), RealmID: "123",
	})

	_, err := c.query(context.Background(), "SELECT * FROM Item")
	if !errors.Is(err, errAuthExpired) {
		t.Fatalf("err = %v, want errAuthExpired", err)
	}
	if f.apiCalls != 0 {
		t.Errorf("apiCalls=%d, want 0 when the refresh fails", f.apiCalls)
	}
}
