package main

import (
	"github.com/alecthomas/kong"
	"github.com/OPPF-IHQ-IT/invoicer/internal/cli"
)

func main() {
	ctx := kong.Parse(&cli.Root,
		kong.Name("invoicer"),
		kong.Description("Automate fraternity dues invoicing via Airtable and QuickBooks Online."),
		kong.UsageOnError(),
	)
	ctx.FatalIfErrorf(ctx.Run(&cli.Globals{ConfigFile: cli.Root.ConfigFile}))
}
