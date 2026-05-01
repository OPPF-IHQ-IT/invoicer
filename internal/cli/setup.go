package cli

import (
	"fmt"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/setup"
)

type SetupCmd struct {
	Run   SetupRunCmd   `cmd:"" help:"Interactive setup wizard — create your config file." default:"withargs"`
	Items SetupItemsCmd `cmd:"" help:"Interactively map QBO products to dues components."`
}

type SetupRunCmd struct{}
type SetupItemsCmd struct{}

func (s *SetupRunCmd) Run(_ *Globals) error {
	return setup.Run()
}

func (s *SetupItemsCmd) Run(globals *Globals) error {
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	return setup.RunItems(cfg)
}
