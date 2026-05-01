package cli

import (
	"fmt"

	"github.com/willmadison/invoicer/internal/config"
)

type ConfigCmd struct {
	Validate ValidateConfigCmd `cmd:"" help:"Validate the config file."`
}

type ValidateConfigCmd struct{}

type AirtableCmd struct {
	Doctor AirtableDoctorCmd `cmd:"" help:"Check Airtable connectivity and schema."`
}

type AirtableDoctorCmd struct{}

type QboCmd struct {
	Doctor QboDoctorCmd `cmd:"" help:"Check QBO connectivity and auth status."`
}

type QboDoctorCmd struct{}

func (v *ValidateConfigCmd) Run(globals *Globals) error {
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return fmt.Errorf("config is invalid: %w", err)
	}
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
	fmt.Println("Config is valid.")
	return nil
}

func (a *AirtableDoctorCmd) Run(globals *Globals) error {
	fmt.Println("airtable doctor: not yet implemented")
	return nil
}

func (q *QboDoctorCmd) Run(globals *Globals) error {
	fmt.Println("qbo doctor: not yet implemented")
	return nil
}
