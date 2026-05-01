package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/willmadison/invoicer/internal/config"
	"github.com/willmadison/invoicer/internal/fiscal"
	"github.com/willmadison/invoicer/internal/planner"
	"github.com/willmadison/invoicer/internal/reports"
	"github.com/willmadison/invoicer/internal/runner"
)

type RunCmd struct {
	FiscalYear       string `short:"f" help:"Fiscal year to run (e.g. FY2026 or 2026)." required:""`
	AsOf             string `help:"Override today's date for date-sensitive logic (YYYY-MM-DD)."`
	CreateOnly       bool   `help:"Create invoices in QBO without sending them."`
	Send             bool   `help:"Send previously-created invoices after manual review."`
	AllowNameFallback bool  `help:"Allow full-name customer matching during run (use with caution)."`
	Env              string `help:"QBO environment." enum:"sandbox,production" default:""`
}

func (r *RunCmd) Validate() error {
	if !r.CreateOnly && !r.Send {
		return fmt.Errorf("must specify --create-only or --send; run 'invoicer run --help' for usage")
	}
	if r.CreateOnly && r.Send {
		return fmt.Errorf("--create-only and --send are mutually exclusive; use one at a time")
	}
	return nil
}

func (r *RunCmd) Run(globals *Globals) error {
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if r.Env != "" {
		cfg.QBO.Environment = r.Env
	}

	if r.AllowNameFallback {
		cfg.Invoice.AllowNameFallbackInRun = true
	}

	fy, err := fiscal.Parse(r.FiscalYear)
	if err != nil {
		return fmt.Errorf("invalid fiscal year %q: %w", r.FiscalYear, err)
	}

	asOf := time.Now().Local().Truncate(24 * time.Hour)
	if r.AsOf != "" {
		asOf, err = time.ParseInLocation("2006-01-02", r.AsOf, time.Local)
		if err != nil {
			return fmt.Errorf("invalid --as-of date %q: %w", r.AsOf, err)
		}
	}

	plan, err := planner.Build(context.Background(), cfg, fy, asOf, r.Send)
	if err != nil {
		return fmt.Errorf("building run plan: %w", err)
	}

	var result *runner.Result
	if r.CreateOnly {
		result, err = runner.CreateInvoices(context.Background(), cfg, plan)
	} else {
		result, err = runner.SendInvoices(context.Background(), cfg, plan)
	}
	if err != nil {
		return err
	}

	return reports.Write(result.Plan, reports.Options{
		Mode:    reports.ModeRun,
		Format:  "json",
		RunID:   result.RunID,
	})
}
