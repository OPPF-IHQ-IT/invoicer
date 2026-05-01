package cli

import "github.com/OPPF-IHQ-IT/invoicer/internal/setup"

type SetupCmd struct{}

func (s *SetupCmd) Run(_ *Globals) error {
	return setup.Run()
}
