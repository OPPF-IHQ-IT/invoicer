package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/duesrun"
	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
)

// DuesRunCmd is the guided annual run.
type DuesRunCmd struct {
	Start  DuesRunStartCmd  `cmd:"" default:"withargs" help:"Start or resume the guided dues run."`
	Status DuesRunStatusCmd `cmd:"" help:"Show where the fiscal year's dues stand, including bounces."`
}

// DuesRunStartCmd walks through pre-flight, review, create, and send.
type DuesRunStartCmd struct {
	FiscalYear string `short:"f" help:"Fiscal year to run (e.g. FY2028)." required:""`
	AsOf       string `help:"Invoice date, and the date for late fees (YYYY-MM-DD). Defaults to the first day of the fiscal year."`
	Env        string `help:"QBO environment override." enum:",sandbox,production" default:""`
	DryRun     bool   `help:"Walk through every step with real data and real prompts, but change nothing."`
	AllMembers bool   `help:"With --dry-run, also include Invoiced and Active members, to practice before statuses are reset for the new year."`
}

// Validate rejects --all-members on a real run, where it would bill everyone.
func (c *DuesRunStartCmd) Validate() error {
	if c.AllMembers && !c.DryRun {
		return fmt.Errorf("--all-members only works with --dry-run")
	}
	return nil
}

// DuesRunStatusCmd reports on a fiscal year.
type DuesRunStatusCmd struct {
	FiscalYear string `short:"f" help:"Fiscal year (e.g. FY2028)." required:""`
	Env        string `help:"QBO environment override." enum:",sandbox,production" default:""`
}

// PreflightCmd runs the checks on their own.
type PreflightCmd struct {
	FiscalYear string `short:"f" help:"Fiscal year to check (e.g. FY2028)." required:""`
	AsOf       string `help:"Invoice date, and the date for late fees (YYYY-MM-DD). Defaults to the first day of the fiscal year."`
	Env        string `help:"QBO environment override." enum:",sandbox,production" default:""`
}

func loadRun(globals *Globals, env, fyArg, asOfArg string) (*config.Config, fiscal.Year, time.Time, error) {
	var asOf time.Time
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return nil, fiscal.Year{}, asOf, fmt.Errorf("loading config: %w", err)
	}
	if err := config.Validate(cfg); err != nil {
		return nil, fiscal.Year{}, asOf, fmt.Errorf("config isn't ready:\n  %w", err)
	}
	if env != "" {
		cfg.QBO.Environment = env
	}
	fy, err := fiscal.Parse(fyArg)
	if err != nil {
		return nil, fiscal.Year{}, asOf, fmt.Errorf("invalid fiscal year %q: %w", fyArg, err)
	}
	if asOfArg == "" {
		asOf = duesrun.DefaultInvoiceDate(fy.PeriodStart)
	} else if asOf, err = time.ParseInLocation("2006-01-02", asOfArg, time.Local); err != nil {
		return nil, fiscal.Year{}, asOf, fmt.Errorf("invalid --as-of date %q: %w", asOfArg, err)
	}
	return cfg, fy, asOf, nil
}

func (c *PreflightCmd) Run(globals *Globals) error {
	cfg, fy, asOf, err := loadRun(globals, c.Env, c.FiscalYear, c.AsOf)
	if err != nil {
		return err
	}
	ctx := context.Background()
	fmt.Printf("Checking %s, dated %s. This changes nothing.\n", fy.Label, asOf.Format("2006-01-02"))
	snap, err := duesrun.Gather(ctx, cfg, fy, asOf)
	if err != nil {
		return err
	}
	findings := duesrun.Check(cfg, snap)
	dir, err := duesrun.RunDir(cfg.App.ReportDir, fy.Label)
	if err != nil {
		return err
	}
	path, err := duesrun.WriteReviewFile(dir, snap, findings)
	if err != nil {
		return err
	}

	s := duesrun.Summarize(snap.Plan)
	fmt.Printf("Connected to %s (%s).\n", snap.Company, snap.Env)
	fmt.Printf("Plan: %d invoices ($%.2f), %d $0 receipts, %d skipped, %d errors.\n\n", s.Invoices, s.Billed, s.Receipts, s.Skipped, s.Errors)
	duesrun.PrintFindings(os.Stdout, findings, "")
	blockers := duesrun.CountByLevel(findings, duesrun.Blocker)
	fmt.Printf("\n%d blockers, %d warnings.\nReview file: %s\n", blockers, duesrun.CountByLevel(findings, duesrun.Warning), path)
	if blockers > 0 {
		return fmt.Errorf("%d blockers must be fixed before a run", blockers)
	}
	return nil
}

func (c *DuesRunStartCmd) Run(globals *Globals) error {
	cfg, fy, asOf, err := loadRun(globals, c.Env, c.FiscalYear, c.AsOf)
	if err != nil {
		return err
	}
	if cfg.QBO.Environment != "production" && c.Env == "" && !c.DryRun {
		return fmt.Errorf("the config points at %s. Pass --env %s if that's intended", cfg.QBO.Environment, cfg.QBO.Environment)
	}
	if c.AllMembers {
		cfg.Airtable.InvoiceableStatuses = append(cfg.Airtable.InvoiceableStatuses, cfg.Airtable.StatusValues.Invoiced, cfg.Airtable.StatusValues.Active)
	}

	label := fy.Label
	var ops duesrun.Ops = &duesrun.LiveOps{Cfg: cfg, FY: fy, AsOf: asOf}
	if c.DryRun {
		// A separate folder, so a practice approval can't satisfy a real run.
		label = fy.Label + "-dry-run"
		ops = &duesrun.DryRunOps{GatherFn: func(ctx context.Context) (*duesrun.Snapshot, error) {
			return duesrun.Gather(ctx, cfg, fy, asOf)
		}}
	}
	dir, err := duesrun.RunDir(cfg.App.ReportDir, label)
	if err != nil {
		return err
	}
	flow := &duesrun.Flow{
		Cfg:    cfg,
		Ops:    ops,
		Dir:    dir,
		In:     os.Stdin,
		Out:    os.Stdout,
		DryRun: c.DryRun,
	}
	err = flow.Run(context.Background())
	if errors.Is(err, duesrun.ErrStopped) {
		return nil
	}
	return err
}

func (c *DuesRunStatusCmd) Run(globals *Globals) error {
	cfg, fy, _, err := loadRun(globals, c.Env, c.FiscalYear, "")
	if err != nil {
		return err
	}
	st, err := duesrun.LoadStatus(context.Background(), cfg, fy)
	if err != nil {
		return err
	}
	fmt.Printf("%s dues in QBO\n\n", fy.Label)
	fmt.Printf("Invoices:    %d, $%.2f billed\n", st.Invoices, st.Billed)
	fmt.Printf("Paid:        %d\n", st.Paid)
	fmt.Printf("Open:        %d, $%.2f outstanding\n", st.Open, st.Outstanding)
	fmt.Printf("Not emailed: %d\n", st.Unsent)
	fmt.Printf("Voided:      %d\n", st.Voided)
	fmt.Printf("$0 receipts: %d (%d not emailed)\n", st.Receipts, st.ReceiptsUnsent)
	fmt.Printf("\nBounces: %d\n", len(st.Bounces))
	for _, b := range st.Bounces {
		fmt.Printf("  %s, %s, $%.2f, %s (%s)\n", b.Customer, b.Document, b.Amount, b.Email, b.Reason)
	}
	if len(st.Bounces) > 0 {
		fmt.Println("\nA bounced document still shows as sent, so reruns skip it. Get a good address, fix it on the customer and the document, and resend from QBO.")
	}
	return nil
}
