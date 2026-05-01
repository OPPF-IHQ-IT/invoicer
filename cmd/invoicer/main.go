package main

import (
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/OPPF-IHQ-IT/invoicer/internal/cli"
	"github.com/OPPF-IHQ-IT/invoicer/internal/setup"
)

func main() {
	if firstRun() {
		if err := setup.Run(); err != nil {
			os.Stderr.WriteString("setup error: " + err.Error() + "\n")
			os.Exit(1)
		}
		return
	}

	ctx := kong.Parse(&cli.Root,
		kong.Name("invoicer"),
		kong.Description("Automate fraternity dues invoicing via Airtable and QuickBooks Online."),
		kong.UsageOnError(),
	)
	ctx.FatalIfErrorf(ctx.Run(&cli.Globals{ConfigFile: cli.Root.ConfigFile}))
}

func firstRun() bool {
	// Only trigger setup when invoked with no arguments — explicit subcommands
	// (including `invoicer setup`) always go through the normal parse path.
	if len(os.Args) > 1 {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".config", "invoicer", "config.yaml"))
	return os.IsNotExist(err)
}
