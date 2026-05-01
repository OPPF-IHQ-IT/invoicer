package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
	"github.com/OPPF-IHQ-IT/invoicer/internal/reports"
)

type PreviewCmd struct {
	FiscalYear string `short:"f" help:"Fiscal year to preview (e.g. FY2026 or 2026)." required:""`
	AsOf       string `help:"Override today's date for date-sensitive logic (YYYY-MM-DD)."`
	Send       bool   `help:"Preview which invoices would be sent."`
	Format     string `help:"Output format." enum:"table,json" default:"table"`
	Out        string `help:"Write report to this file path." type:"path"`
	Env        string `help:"QBO environment override." enum:",sandbox,production" default:""`
}

func (p *PreviewCmd) Run(globals *Globals) error {
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if p.Env != "" {
		cfg.QBO.Environment = p.Env
	}

	fy, err := fiscal.Parse(p.FiscalYear)
	if err != nil {
		return fmt.Errorf("invalid fiscal year %q: %w", p.FiscalYear, err)
	}

	now := time.Now().Local()
	asOf := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if p.AsOf != "" {
		asOf, err = time.ParseInLocation("2006-01-02", p.AsOf, time.Local)
		if err != nil {
			return fmt.Errorf("invalid --as-of date %q: %w", p.AsOf, err)
		}
	}

	plan, err := planner.Build(context.Background(), cfg, fy, asOf, p.Send)
	if err != nil {
		return fmt.Errorf("building preview plan: %w", err)
	}

	return reports.Write(plan, reports.Options{
		Mode:   reports.ModePreview,
		Format: p.Format,
		OutFile: p.Out,
	})
}
