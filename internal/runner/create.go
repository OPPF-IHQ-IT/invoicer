package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// Result wraps the mutated plan and a run ID after execution.
type Result struct {
	Plan  *planner.Plan
	RunID string
}

// CreateInvoices executes the create-only phase: creates QBO invoices and updates Airtable status.
func CreateInvoices(ctx context.Context, cfg *config.Config, plan *planner.Plan) (*Result, error) {
	qboClient, err := qbo.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	atClient := airtable.NewClient(cfg.Airtable.APIKey, cfg.Airtable.BaseID)

	runID := time.Now().UTC().Format("2006-01-02T150405Z")

	for i := range plan.MemberPlans {
		mp := &plan.MemberPlans[i]
		if mp.Action != planner.ActionCreate {
			continue
		}

		privateNote := planner.BuildPrivateNote(plan.FY, mp.Member.RecordID, mp.CalcResult.LineHash, runID)

		inv, err := qboClient.CreateInvoice(ctx, qbo.InvoiceCreateRequest{
			CustomerRef: qbo.CustomerRef{Value: mp.QBOCustomer.ID, Name: mp.QBOCustomer.DisplayName},
			TxnDate:     mp.InvoiceDate,
			DueDate:     mp.DueDate,
			PrivateNote: privateNote,
			Line:        mp.CalcResult.Lines,
		})
		if err != nil {
			mp.Action = planner.ActionError
			mp.SkipDetail = fmt.Sprintf("creating QBO invoice: %v", err)
			continue
		}

		mp.ExistingInvoice = inv

		// Update Airtable member status.
		newStatus := cfg.Airtable.StatusValues.Invoiced
		if mp.CalcResult.IsZeroTotal {
			newStatus = cfg.Airtable.StatusValues.Active
		}
		if err := atClient.UpdateMemberStatus(ctx, &cfg.Airtable, mp.Member.RecordID, newStatus); err != nil {
			// Non-fatal: log but don't fail the whole run.
			mp.SkipDetail = fmt.Sprintf("invoice created (%s) but Airtable status update failed: %v", inv.ID, err)
		}

		// Consume poll worker credits if any were applied.
		if err := consumePollWorkerCredits(ctx, atClient, cfg, mp); err != nil {
			mp.SkipDetail += fmt.Sprintf("; poll worker credit update failed: %v", err)
		}
	}

	return &Result{Plan: plan, RunID: runID}, nil
}

func consumePollWorkerCredits(ctx context.Context, atClient *airtable.Client, cfg *config.Config, mp *planner.MemberPlan) error {
	if mp.CalcResult == nil {
		return nil
	}

	// Find the poll worker credit line item to determine how much was applied.
	var creditApplied float64
	for _, line := range mp.CalcResult.Lines {
		if line.SalesItemLineDetail != nil &&
			line.SalesItemLineDetail.ItemRef.Value == cfg.QBOItems.PollWorkerCredit {
			creditApplied += -line.Amount // credits are negative amounts
		}
	}
	if creditApplied <= 0 {
		return nil
	}

	credits, err := atClient.ListPollWorkerCredits(ctx, &cfg.Airtable, mp.Member.RecordID)
	if err != nil {
		return err
	}

	// FIFO: consume from oldest rows first.
	remaining := creditApplied
	amounts := make([]float64, len(credits))
	for i, c := range credits {
		if remaining <= 0 {
			break
		}
		consume := c.CreditsAvailable
		if consume > remaining {
			consume = remaining
		}
		amounts[i] = consume
		remaining -= consume
	}

	return atClient.ConsumePollWorkerCredits(ctx, &cfg.Airtable, credits, amounts)
}
