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
	authURL = "https://appcenter.intuit.com/connect/oauth2"
	scope   = "com.intuit.quickbooks.accounting"
)

// tokenURL is a variable so tests can point it at a local server.
var tokenURL = "https://oauth.platform.intuit.com/oauth2/v1/tokens/bearer"

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
	tokenResp, err := requestToken(ctx, cfg, url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	})
	if err != nil {
		return err
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

// refreshAccessToken trades the stored refresh token for a new access token and
// saves it. Intuit access tokens last about an hour; refresh tokens last about
// 100 days, so a refresh avoids sending the user back through the browser flow.
func refreshAccessToken(ctx context.Context, cfg *config.Config, tok *storage.Token) (*storage.Token, error) {
	if tok.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token stored")
	}
	tokenResp, err := requestToken(ctx, cfg, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
	})
	if err != nil {
		return nil, err
	}

	refreshed := &storage.Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Unix(),
		RealmID:      tok.RealmID,
	}
	// Intuit may rotate the refresh token; keep the old one if none came back.
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = tok.RefreshToken
	}
	if err := storage.SaveToken(cfg, refreshed); err != nil {
		return nil, fmt.Errorf("saving refreshed token: %w", err)
	}
	return refreshed, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// requestToken posts a grant to Intuit's token endpoint.
func requestToken(ctx context.Context, cfg *config.Config, form url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cfg.QBO.ClientID, cfg.QBO.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed (status %d)", resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}
	return &tokenResp, nil
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
		if tok.RefreshToken != "" {
			status = "expired (refreshes automatically on next use)"
		}
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
