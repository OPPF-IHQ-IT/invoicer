package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/willmadison/invoicer/internal/config"
)

// Token holds persisted QBO OAuth credentials.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresAt    int64  `json:"expires_at"`
	RealmID      string `json:"realm_id"`
}

func tokenPath(cfg *config.Config) string {
	return filepath.Join(config.TokenDir(), "qbo-token.json")
}

// LoadToken reads the persisted QBO token.
func LoadToken(cfg *config.Config) (*Token, error) {
	path := tokenPath(cfg)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no QBO token found at %s", path)
		}
		return nil, err
	}

	var tok Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("parsing QBO token: %w", err)
	}
	return &tok, nil
}

// SaveToken writes the QBO token with restrictive permissions.
func SaveToken(cfg *config.Config, tok *Token) error {
	dir := config.TokenDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}

	path := tokenPath(cfg)
	// Write to temp file then rename for atomicity.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DeleteToken removes the persisted QBO token.
func DeleteToken(cfg *config.Config) error {
	path := tokenPath(cfg)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
