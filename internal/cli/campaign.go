package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/OPPF-IHQ-IT/invoicer/internal/campaign"
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
)

type CampaignCmd struct {
	CSV string `arg:"" help:"Path to the Google Form CSV export." type:"existingfile"`

	ItemID       string `help:"QBO Item ID to invoice against." required:""`
	Name         string `help:"Campaign name; used as line description and in the invoice PrivateNote."`
	Memo         string `help:"Override invoice.customer_memo for this run."`
	DryRun       bool   `help:"Reconcile only; do not create or send invoices." default:"true" negatable:""`
	Yes          bool   `help:"Required to actually create+send when --no-dry-run is set."`
	CreateMissing bool  `help:"Create QBO customers for matched Airtable records that lack a QBO Customer ID; writes ID back to Airtable." default:"true" negatable:""`
	MatchedOut   string `help:"Write matched rows to this CSV file." type:"path"`
	UnmatchedOut string `help:"Write unmatched rows to this CSV file." type:"path"`
	SkippedOut   string `help:"Write skipped rows to this CSV file." type:"path"`
	Out          string `help:"Write full JSON report to this file." type:"path"`
	Env          string `help:"QBO environment override." enum:",sandbox,production" default:""`
}

func (c *CampaignCmd) Validate() error {
	if !c.DryRun && !c.Yes {
		return fmt.Errorf("refusing to create+send invoices without --yes (re-run with --no-dry-run --yes once you've reviewed the dry-run report)")
	}
	return nil
}

func (c *CampaignCmd) Run(globals *Globals) error {
	cfg, err := config.Load(globals.ConfigFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if c.Env != "" {
		cfg.QBO.Environment = c.Env
	}

	ctx := context.Background()

	if err := campaign.VerifyItemID(ctx, cfg, c.ItemID); err != nil {
		return err
	}

	rows, err := campaign.LoadRows(c.CSV)
	if err != nil {
		return err
	}
	superseded := campaign.SupersededRows(rows)
	for _, r := range superseded {
		fmt.Fprintf(os.Stderr, "campaign: superseded earlier submission for control# %s (line %d)\n", r.ControlNumber, r.LineNumber)
	}

	res, err := campaign.Reconcile(ctx, cfg, rows, campaign.Options{CreateMissing: c.CreateMissing})
	if err != nil {
		return err
	}

	reportOpts := campaign.ReportOptions{
		MatchedOut:   c.MatchedOut,
		UnmatchedOut: c.UnmatchedOut,
		SkippedOut:   c.SkippedOut,
		JSONOut:      c.Out,
	}
	if err := campaign.WriteReconciliationCSVs(res, superseded, reportOpts); err != nil {
		return err
	}

	if c.DryRun {
		summary := campaign.CountsFromRun(res, superseded, nil, c.Name)
		campaign.PrintSummary(os.Stdout, summary, false)
		if c.Out != "" {
			report := campaign.JSONReport{
				CampaignName: c.Name,
				DryRun:       true,
				ItemID:       c.ItemID,
				Counts:       summary,
				Matched:      res.Matched,
				Unmatched:    res.Unmatched,
				Skipped:      res.Skipped,
				Superseded:   superseded,
			}
			if err := campaign.WriteJSONReport(c.Out, report); err != nil {
				return err
			}
		}
		return nil
	}

	runResult, err := campaign.Execute(ctx, cfg, res.Matched, campaign.RunOptions{
		ItemID:       c.ItemID,
		CampaignName: c.Name,
		CustomerMemo: c.Memo,
	})
	if err != nil {
		return err
	}

	summary := campaign.CountsFromRun(res, superseded, runResult, c.Name)
	campaign.PrintSummary(os.Stdout, summary, true)

	if c.Out != "" {
		report := campaign.JSONReport{
			CampaignName: c.Name,
			RunID:        runResult.RunID,
			DryRun:       false,
			ItemID:       c.ItemID,
			Counts:       summary,
			Matched:      res.Matched,
			Unmatched:    res.Unmatched,
			Skipped:      res.Skipped,
			Superseded:   superseded,
			Outcomes:     runResult.Outcomes,
		}
		if err := campaign.WriteJSONReport(c.Out, report); err != nil {
			return err
		}
	}

	return nil
}
