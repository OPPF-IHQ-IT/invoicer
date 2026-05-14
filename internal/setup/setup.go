package setup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const configTemplate = `app:
  default_environment: {{ .Environment }}
  report_dir: ./.invoicer-runs

fiscal_year:
  start_month: 11
  start_day: 1
  end_month: 10
  end_day: 31
  airtable_format: "YYYY-YYYY"

qbo:
  client_id: "{{ .QBOClientID }}"
  client_secret: "{{ .QBOClientSecret }}"
  redirect_host: localhost
  redirect_port: 8484
{{- if eq .Environment "production" }}
  redirect_uri: "https://invoicer.wmadisondev.workers.dev/callback"
{{- end }}
  environment: {{ .Environment }}
  sandbox_base_url: "https://sandbox-quickbooks.api.intuit.com"
  production_base_url: "https://quickbooks.api.intuit.com"

# QBO Item IDs — run 'invoicer qbo doctor' after authenticating to list available items,
# then fill in the IDs below.
qbo_items:
  international: ""
  district: ""
  state: ""
  local: ""
  local_retiree: ""
  local_late_fee: ""
  international_life_membership: ""
  district_life_membership: ""
  state_life_membership: ""
  local_life_membership: ""
  local_life_membership_retiree: ""
  basileus_emeritus_offset: ""
  basileus_emeritus_offset_retiree: ""
  international_msp: ""
  district_msp: ""
  state_msp: ""
  local_msp: ""
  local_msp_retiree: ""
  poll_worker_credit: ""
  poll_worker_credit_unit: 50
  international_reinstatement: ""

airtable:
  api_key: "{{ .AirtableAPIKey }}"
  base_id: "{{ .AirtableBaseID }}"

  tables:
    members: "Members"
    dues_records: "Dues Records"
    dues_schedule: "Dues Schedule"
    poll_worker_credit_utilization: "Poll Worker Dues Credit Utilization"

  views:
    members: "Internal Administrative Roster"
    dues_records: "Grid view"
    dues_schedule: "Grid view"

  invoiceable_statuses:
    - "Invoicable"
    - "Reclaimable"

  status_values:
    invoiceable: "Invoicable"
    reclaimable: "Reclaimable"
    invoiced: "Invoiced"
    active: "Active"
    no_longer_member: "No Longer in Chi Tau"

  fields:
    members:
      control_number: "Control #"
      email: "Email"
      status: "Status"
      qbo_customer_id: "QBO Customer ID"
      intl_life: "Intl Life?"
      district_life: "District Life?"
      state_life: "State Life?"
      local_life: "Chi Tau Life?"
      basileus_emeritus: "Basileus Emeritus?"
      retired: "Retired?"
      recent_msp: "Recent MSP?"

    dues_schedule:
      id: "ID"
      fiscal_year: "Fiscal Year"
      level: "Level"
      amount: "Amount"
      due_date: "Due Date"
      key: "Key"

    poll_worker_credits:
      member_link: "Members"
      fiscal_year: "Fiscal Year"
      credits_earned: "Poll Worker Dues Credits Earned"
      credits_spent: "Poll Worker Dues Credits Spent"
      credits_available: "Poll Worker Dues Credits Available"

invoice:
  due_on_receipt: true
  auto_number: true
  allow_name_fallback_in_run: false
  create_missing_customers: false
  default_run_mode_requires_explicit_action: true
  customer_memo: ""
  # QBO Term entity ID stamped on every invoice (e.g. "Due on Receipt"). Find it
  # in QBO under Settings → All lists → Terms; the ID is in the URL when
  # editing the term. Leave blank to omit the Terms reference.
  sales_term_id: ""
`

type configValues struct {
	QBOClientID     string
	QBOClientSecret string
	Environment     string
	AirtableAPIKey  string
	AirtableBaseID  string
}

// Run executes the interactive setup flow and writes ~/.config/invoicer/config.yaml.
func Run() error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println()
	fmt.Println("Welcome to invoicer setup!")
	fmt.Println("This will walk you through creating your config file.")
	fmt.Println()

	configPath := filepath.Join(mustHomeDir(), ".config", "invoicer", "config.yaml")

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("A config file already exists at %s\n", configPath)
		if !confirm(scanner, "Overwrite it?", false) {
			fmt.Println("Setup cancelled. Your existing config was not changed.")
			return nil
		}
		fmt.Println()
	}

	vals := configValues{}

	// QBO environment
	fmt.Println("─── QuickBooks Online ───────────────────────────────────────")
	fmt.Println()
	fmt.Println("Are you setting up for sandbox (testing) or production (real invoices)?")
	vals.Environment = promptEnum(scanner, "Environment", []string{"sandbox", "production"}, "production")
	fmt.Println()
	fmt.Println("You'll need a QBO app client ID and secret from the Intuit Developer portal.")
	fmt.Println("If you don't have these yet, visit: https://developer.intuit.com")
	fmt.Println()
	vals.QBOClientID = promptRequired(scanner, "QBO Client ID")
	vals.QBOClientSecret = promptRequired(scanner, "QBO Client Secret")
	fmt.Println()

	// Airtable
	fmt.Println("─── Airtable ────────────────────────────────────────────────")
	fmt.Println()
	fmt.Println("Your Airtable API key can be found at: https://airtable.com/account")
	fmt.Println()
	vals.AirtableAPIKey = promptRequired(scanner, "Airtable API Key")
	fmt.Println()
	fmt.Println("Your Airtable Base ID is in the URL when you open your base:")
	fmt.Println("  https://airtable.com/appXXXXXXXXXXXXXX/...")
	fmt.Println("                        ^^^^^^^^^^^^^^^^ this part")
	fmt.Println()
	vals.AirtableBaseID = promptRequired(scanner, "Airtable Base ID")
	fmt.Println()

	// Write config
	if err := writeConfig(configPath, vals); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Println()
	fmt.Printf("Config written to %s\n", configPath)
	fmt.Println()
	fmt.Println("─── Next steps ──────────────────────────────────────────────")
	fmt.Println()
	fmt.Println("1. Authenticate with QuickBooks Online:")
	fmt.Printf("     invoicer auth login --env=%s\n", vals.Environment)
	fmt.Println()
	fmt.Println("2. List your QBO products to fill in item IDs:")
	fmt.Println("     invoicer qbo doctor")
	fmt.Printf("   Then edit %s and fill in the qbo_items section.\n", configPath)
	fmt.Println()
	fmt.Println("3. Reconcile your QBO customers against Airtable members:")
	fmt.Println("     invoicer customers reconcile")
	fmt.Println()
	fmt.Println("4. Preview what would be invoiced:")
	fmt.Println("     invoicer preview --fiscal-year FY2026")
	fmt.Println()

	return nil
}

func writeConfig(path string, vals configValues) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return err
	}
	return tmpl.Execute(f, vals)
}

func promptRequired(scanner *bufio.Scanner, label string) string {
	for {
		fmt.Printf("%s: ", label)
		scanner.Scan()
		val := strings.TrimSpace(scanner.Text())
		if val != "" {
			return val
		}
		fmt.Println("  This value is required.")
	}
}

func promptEnum(scanner *bufio.Scanner, label string, options []string, defaultVal string) string {
	display := strings.Join(options, "/")
	for {
		fmt.Printf("%s (%s) [%s]: ", label, display, defaultVal)
		scanner.Scan()
		val := strings.TrimSpace(scanner.Text())
		if val == "" {
			return defaultVal
		}
		for _, o := range options {
			if strings.EqualFold(val, o) {
				return o
			}
		}
		fmt.Printf("  Please enter one of: %s\n", display)
	}
}

func confirm(scanner *bufio.Scanner, label string, defaultVal bool) bool {
	defaultStr := "y/N"
	if defaultVal {
		defaultStr = "Y/n"
	}
	fmt.Printf("%s [%s]: ", label, defaultStr)
	scanner.Scan()
	val := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if val == "" {
		return defaultVal
	}
	return val == "y" || val == "yes"
}

func mustHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return home
}

