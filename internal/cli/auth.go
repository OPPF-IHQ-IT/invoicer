package cli

import (
	"context"
	"fmt"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

type AuthCmd struct {
	Login  AuthLoginCmd `cmd:"" help:"Authenticate with QuickBooks Online." default:"withargs"`
	Status StatusCmd    `cmd:"" help:"Show current authentication status."`
	Logout LogoutCmd    `cmd:"" help:"Remove stored QBO credentials."`
}

type AuthLoginCmd struct {
	Env string `help:"QBO environment to authenticate against." enum:"sandbox,production" default:"sandbox"`
}

type StatusCmd struct{}
type LogoutCmd struct{}

func (a *AuthLoginCmd) Run(globals *Globals) error {
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if a.Env != "" {
		cfg.QBO.Environment = a.Env
	}

	return qbo.RunOAuthFlow(context.Background(), cfg)
}

func (s *StatusCmd) Run(globals *Globals) error {
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	return qbo.ShowAuthStatus(cfg)
}

func (l *LogoutCmd) Run(globals *Globals) error {
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	return qbo.Logout(cfg)
}
