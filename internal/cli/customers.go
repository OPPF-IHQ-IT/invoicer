package cli

import (
	"context"
	"fmt"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/reconcile"
)

type CustomersCmd struct {
	Reconcile ReconcileCmd `cmd:"" help:"Reconcile QBO customers against Airtable members."`
}

type ReconcileCmd struct {
	DryRun         bool   `help:"Preview matches without updating Airtable." default:"true" negatable:""`
	UpdateAirtable bool   `help:"Write matched QBO Customer IDs back to Airtable."`
	Overwrite      bool   `help:"Overwrite existing QBO Customer ID values in Airtable."`
	CreateMissing  bool   `help:"Create QBO customer records for unmatched members who have an email address."`
	AmbiguousOut   string `help:"Write ambiguous matches to this CSV file." type:"path"`
	MatchedOut     string `help:"Write matched records to this CSV file." type:"path"`
	UnmatchedOut   string `help:"Write unmatched records to this CSV file." type:"path"`
	SkippedOut     string `help:"Write skipped records (e.g. missing email) to this CSV file." type:"path"`
}

func (r *ReconcileCmd) Run(globals *Globals) error {
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	opts := reconcile.Options{
		DryRun:         r.DryRun,
		UpdateAirtable: r.UpdateAirtable,
		Overwrite:      r.Overwrite,
		CreateMissing:  r.CreateMissing,
		AmbiguousOut:   r.AmbiguousOut,
		MatchedOut:     r.MatchedOut,
		UnmatchedOut:   r.UnmatchedOut,
		SkippedOut:     r.SkippedOut,
	}

	return reconcile.Customers(context.Background(), cfg, opts)
}
