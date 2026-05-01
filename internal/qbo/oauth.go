package qbo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/storage"
)

const (
	authURL  = "https://appcenter.intuit.com/connect/oauth2"
	tokenURL = "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer"
	scope    = "com.intuit.quickbooks.accounting"
)

// RunOAuthFlow starts the localhost loopback OAuth 2.0 flow for QBO.
func RunOAuthFlow(ctx context.Context, cfg *config.Config) error {
	state, err := randomState()
	if err != nil {
		return err
	}

	redirectURI := cfg.QBO.OAuthRedirectURI()

	authParams := url.Values{
		"client_id":     {cfg.QBO.ClientID},
		"response_type": {"code"},
		"scope":         {scope},
		"redirect_uri":  {redirectURI},
		"state":         {state},
	}
	authLink := authURL + "?" + authParams.Encode()

	codeCh := make(chan callbackResult, 1)
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.QBO.RedirectPort)}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			codeCh <- callbackResult{err: fmt.Errorf("state mismatch: possible CSRF")}
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if errMsg := q.Get("error"); errMsg != "" {
			codeCh <- callbackResult{err: fmt.Errorf("QBO auth error: %s", errMsg)}
			fmt.Fprintf(w, "<html><body><p>Authentication failed: %s. You may close this window.</p></body></html>", errMsg)
			return
		}
		codeCh <- callbackResult{code: q.Get("code"), realmID: q.Get("realmId")}
		fmt.Fprint(w, "<html><body><p>Authentication successful. You may close this window.</p></body></html>")
	})

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.QBO.RedirectPort))
	if err != nil {
		return fmt.Errorf("starting callback server on port %d: %w", cfg.QBO.RedirectPort, err)
	}

	go srv.Serve(listener) //nolint:errcheck

	fmt.Printf("Opening browser for QBO authorization...\nIf your browser does not open, visit:\n\n  %s\n\n", authLink)
	openBrowser(authLink)

	select {
	case result := <-codeCh:
		srv.Shutdown(context.Background()) //nolint:errcheck
		if result.err != nil {
			return result.err
		}
		return exchangeCode(ctx, cfg, result.code, result.realmID, redirectURI)
	case <-time.After(5 * time.Minute):
		srv.Shutdown(context.Background()) //nolint:errcheck
		return fmt.Errorf("authentication timed out after 5 minutes")
	case <-ctx.Done():
		srv.Shutdown(context.Background()) //nolint:errcheck
		return ctx.Err()
	}
}

type callbackResult struct {
	code    string
	realmID string
	err     error
}

func exchangeCode(ctx context.Context, cfg *config.Config, code, realmID, redirectURI string) error {
	body := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(cfg.QBO.ClientID, cfg.QBO.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token exchange failed (status %d)", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return fmt.Errorf("parsing token response: %w", err)
	}

	tok := &storage.Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Unix(),
		RealmID:      realmID,
	}
	if err := storage.SaveToken(cfg, tok); err != nil {
		return fmt.Errorf("saving token: %w", err)
	}

	fmt.Printf("Authenticated successfully. Realm ID: %s\n", realmID)
	return nil
}

// ShowAuthStatus prints the current authentication status.
func ShowAuthStatus(cfg *config.Config) error {
	tok, err := storage.LoadToken(cfg)
	if err != nil {
		fmt.Println("Status: not authenticated")
		fmt.Println("Run 'invoicer auth' to authenticate.")
		return nil
	}
	expires := time.Unix(tok.ExpiresAt, 0)
	status := "valid"
	if time.Now().After(expires) {
		status = "expired"
	}
	fmt.Printf("Status: authenticated\nRealm ID: %s\nToken: %s\nExpires: %s\n",
		tok.RealmID, status, expires.Format(time.RFC3339))
	return nil
}

// Logout removes the stored QBO token.
func Logout(cfg *config.Config) error {
	if err := storage.DeleteToken(cfg); err != nil {
		return err
	}
	fmt.Println("Logged out successfully.")
	return nil
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
