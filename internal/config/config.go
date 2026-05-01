package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App        AppConfig        `yaml:"app"`
	FiscalYear FiscalYearConfig `yaml:"fiscal_year"`
	QBO        QBOConfig        `yaml:"qbo"`
	QBOItems   QBOItemsConfig   `yaml:"qbo_items"`
	Airtable   AirtableConfig   `yaml:"airtable"`
	Invoice    InvoiceConfig    `yaml:"invoice"`
}

type AppConfig struct {
	DefaultEnvironment string `yaml:"default_environment"`
	ReportDir          string `yaml:"report_dir"`
}

type FiscalYearConfig struct {
	StartMonth int    `yaml:"start_month"`
	StartDay   int    `yaml:"start_day"`
	EndMonth   int    `yaml:"end_month"`
	EndDay     int    `yaml:"end_day"`
	AirtableFormat string `yaml:"airtable_format"`
}

type QBOConfig struct {
	ClientID           string `yaml:"client_id"`
	ClientSecret       string `yaml:"client_secret"`
	RedirectHost       string `yaml:"redirect_host"`
	RedirectPort       int    `yaml:"redirect_port"`
	Environment        string `yaml:"environment"`
	SandboxBaseURL     string `yaml:"sandbox_base_url"`
	ProductionBaseURL  string `yaml:"production_base_url"`
}

func (q QBOConfig) BaseURL() string {
	if q.Environment == "production" {
		return q.ProductionBaseURL
	}
	return q.SandboxBaseURL
}

// QBOItemsConfig maps dues schedule levels to QBO Item IDs.
// All values are configurable so other chapters can use their own SKUs.
type QBOItemsConfig struct {
	International            string `yaml:"international"`
	District                 string `yaml:"district"`
	State                    string `yaml:"state"`
	Local                    string `yaml:"local"`
	LocalRetiree             string `yaml:"local_retiree"`
	LocalLateFee             string `yaml:"local_late_fee"`
	InternationalLifeMember  string `yaml:"international_life_membership"`
	DistrictLifeMember       string `yaml:"district_life_membership"`
	StateLifeMember          string `yaml:"state_life_membership"`
	LocalLifeMember          string `yaml:"local_life_membership"`
	BasileusEmeritusOffset   string `yaml:"basileus_emeritus_offset"`
	PollWorkerCredit         string `yaml:"poll_worker_credit"`
	InternationalReinstatement string `yaml:"international_reinstatement"`
}

type AirtableConfig struct {
	APIKey  string              `yaml:"api_key"`
	BaseID  string              `yaml:"base_id"`
	Tables  AirtableTablesConfig `yaml:"tables"`
	Views   AirtableViewsConfig  `yaml:"views"`
	Fields  AirtableFieldsConfig `yaml:"fields"`
	InvoiceableStatuses []string `yaml:"invoiceable_statuses"`
	StatusValues        AirtableStatusValuesConfig `yaml:"status_values"`
}

type AirtableTablesConfig struct {
	Members                    string `yaml:"members"`
	DuesRecords                string `yaml:"dues_records"`
	DuesSchedule               string `yaml:"dues_schedule"`
	PollWorkerCreditUtilization string `yaml:"poll_worker_credit_utilization"`
}

type AirtableViewsConfig struct {
	Members      string `yaml:"members"`
	DuesRecords  string `yaml:"dues_records"`
	DuesSchedule string `yaml:"dues_schedule"`
}

type AirtableFieldsConfig struct {
	Members      MembersFieldsConfig      `yaml:"members"`
	DuesSchedule DuesScheduleFieldsConfig `yaml:"dues_schedule"`
	PollWorkerCredits PollWorkerCreditsFieldsConfig `yaml:"poll_worker_credits"`
}

type MembersFieldsConfig struct {
	ControlNumber   string `yaml:"control_number"`
	Email           string `yaml:"email"`
	Status          string `yaml:"status"`
	QBOCustomerID   string `yaml:"qbo_customer_id"`
	IntlLife        string `yaml:"intl_life"`
	DistrictLife    string `yaml:"district_life"`
	StateLife       string `yaml:"state_life"`
	LocalLife       string `yaml:"local_life"`
	BasileusEmeritus string `yaml:"basileus_emeritus"`
	Retired         string `yaml:"retired"`
}

type DuesScheduleFieldsConfig struct {
	ID         string `yaml:"id"`
	FiscalYear string `yaml:"fiscal_year"`
	Level      string `yaml:"level"`
	Amount     string `yaml:"amount"`
	DueDate    string `yaml:"due_date"`
	Key        string `yaml:"key"`
}

type PollWorkerCreditsFieldsConfig struct {
	MemberLink    string `yaml:"member_link"`
	FiscalYear    string `yaml:"fiscal_year"`
	CreditsEarned string `yaml:"credits_earned"`
	CreditsSpent  string `yaml:"credits_spent"`
	CreditsAvailable string `yaml:"credits_available"`
}

type AirtableStatusValuesConfig struct {
	Invoiceable string `yaml:"invoiceable"`
	Invoiced    string `yaml:"invoiced"`
	Active      string `yaml:"active"`
}

type InvoiceConfig struct {
	DueOnReceipt                 bool `yaml:"due_on_receipt"`
	AutoNumber                   bool `yaml:"auto_number"`
	AllowNameFallbackInRun       bool `yaml:"allow_name_fallback_in_run"`
	CreateMissingCustomers       bool `yaml:"create_missing_customers"`
	DefaultRunModeRequiresExplicit bool `yaml:"default_run_mode_requires_explicit_action"`
}

// Load reads and parses the config file, expanding environment variables.
func Load(path string) (*Config, error) {
	path = expandPath(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	applyDefaults(&cfg)
	return &cfg, nil
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func applyDefaults(cfg *Config) {
	if cfg.App.DefaultEnvironment == "" {
		cfg.App.DefaultEnvironment = "sandbox"
	}
	if cfg.App.ReportDir == "" {
		cfg.App.ReportDir = "./.invoicer-runs"
	}
	if cfg.QBO.Environment == "" {
		cfg.QBO.Environment = cfg.App.DefaultEnvironment
	}
	if cfg.QBO.RedirectHost == "" {
		cfg.QBO.RedirectHost = "localhost"
	}
	if cfg.QBO.RedirectPort == 0 {
		cfg.QBO.RedirectPort = 8484
	}
	if cfg.QBO.SandboxBaseURL == "" {
		cfg.QBO.SandboxBaseURL = "https://sandbox-quickbooks.api.intuit.com"
	}
	if cfg.QBO.ProductionBaseURL == "" {
		cfg.QBO.ProductionBaseURL = "https://quickbooks.api.intuit.com"
	}
	if len(cfg.Airtable.InvoiceableStatuses) == 0 {
		cfg.Airtable.InvoiceableStatuses = []string{"Invoicable"}
	}
	if cfg.Airtable.StatusValues.Invoiceable == "" {
		cfg.Airtable.StatusValues.Invoiceable = "Invoicable"
	}
	if cfg.Airtable.StatusValues.Invoiced == "" {
		cfg.Airtable.StatusValues.Invoiced = "Invoiced"
	}
	if cfg.Airtable.StatusValues.Active == "" {
		cfg.Airtable.StatusValues.Active = "Active"
	}
	if cfg.FiscalYear.StartMonth == 0 {
		cfg.FiscalYear.StartMonth = 11
		cfg.FiscalYear.StartDay = 1
		cfg.FiscalYear.EndMonth = 10
		cfg.FiscalYear.EndDay = 31
	}
}
