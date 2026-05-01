package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Validate checks that all required config fields are populated.
func Validate(cfg *Config) error {
	var errs []string

	if cfg.QBO.ClientID == "" {
		errs = append(errs, "qbo.client_id is required (set QBO_CLIENT_ID env var)")
	}
	if cfg.QBO.ClientSecret == "" {
		errs = append(errs, "qbo.client_secret is required (set QBO_CLIENT_SECRET env var)")
	}
	if cfg.Airtable.APIKey == "" {
		errs = append(errs, "airtable.api_key is required (set AIRTABLE_API_KEY env var)")
	}
	if cfg.Airtable.BaseID == "" {
		errs = append(errs, "airtable.base_id is required")
	}
	if cfg.Airtable.Tables.Members == "" {
		errs = append(errs, "airtable.tables.members is required")
	}
	if cfg.Airtable.Tables.DuesSchedule == "" {
		errs = append(errs, "airtable.tables.dues_schedule is required")
	}
	if cfg.Airtable.Tables.PollWorkerCreditUtilization == "" {
		errs = append(errs, "airtable.tables.poll_worker_credit_utilization is required")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n  "))
	}
	return nil
}

// TokenDir returns the directory where QBO tokens are stored.
func TokenDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config/invoicer"
	}
	return fmt.Sprintf("%s/.config/invoicer", home)
}
