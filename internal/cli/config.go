package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
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
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	client, err := qbo.NewClient(cfg)
	if err != nil {
		return err
	}

	items, err := client.ListItems(context.Background())
	if err != nil {
		return fmt.Errorf("listing QBO items: %w", err)
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })

	fmt.Printf("QBO connection OK — %d active items:\n\n", len(items))
	fmt.Printf("%-10s  %-10s  %s\n", "ID", "Type", "Name")
	fmt.Printf("%-10s  %-10s  %s\n", "----------", "----------", "--------------------")
	for _, item := range items {
		fmt.Printf("%-10s  %-10s  %s\n", item.ID, item.Type, item.Name)
	}
	return nil
}
