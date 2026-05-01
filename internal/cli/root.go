package cli

import "github.com/willmadison/invoicer/internal/config"

var Root struct {
	ConfigFile string `short:"c" help:"Path to config file." default:"~/.config/invoicer/config.yaml" type:"path"`

	Auth      AuthCmd      `cmd:"" help:"Authenticate with QuickBooks Online."`
	Preview   PreviewCmd   `cmd:"" help:"Preview what invoicer would create, send, or skip."`
	Run       RunCmd       `cmd:"" help:"Execute invoice creation or sending."`
	Customers CustomersCmd `cmd:"" help:"Customer reconciliation commands."`
	Config    ConfigCmd    `cmd:"" help:"Config management commands."`
	Airtable  AirtableCmd  `cmd:"" help:"Airtable diagnostics."`
	Qbo       QboCmd       `cmd:"" help:"QuickBooks Online diagnostics."`
}

type Globals struct {
	ConfigFile string
	Config     *config.Config
}
