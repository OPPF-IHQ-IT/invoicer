package cli

import (
	"context"
	"fmt"

	"github.com/willmadison/invoicer/internal/config"
	"github.com/willmadison/invoicer/internal/reconcile"
)

type CustomersCmd struct {
	Reconcile ReconcileCmd `cmd:"" help:"Reconcile QBO customers against Airtable members."`
}

type ReconcileCmd struct {
	File          string `short:"f" help:"Path to QBO customers CSV export." required:"" type:"path"`
	DryRun        bool   `help:"Preview matches without updating Airtable." default:"true" negatable:""`
	UpdateAirtable bool  `help:"Write matched QBO Customer IDs back to Airtable."`
	Overwrite     bool   `help:"Overwrite existing QBO Customer ID values in Airtable."`
	AmbiguousOut  string `help:"Write ambiguous matches to this CSV file." type:"path"`
	MatchedOut    string `help:"Write matched records to this CSV file." type:"path"`
	UnmatchedOut  string `help:"Write unmatched records to this CSV file." type:"path"`
}

func (r *ReconcileCmd) Run(globals *Globals) error {
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	opts := reconcile.Options{
		QBOCustomersFile: r.File,
		DryRun:           r.DryRun,
		UpdateAirtable:   r.UpdateAirtable,
		Overwrite:        r.Overwrite,
		AmbiguousOut:     r.AmbiguousOut,
		MatchedOut:       r.MatchedOut,
		UnmatchedOut:     r.UnmatchedOut,
	}

	return reconcile.Customers(context.Background(), cfg, opts)
}
