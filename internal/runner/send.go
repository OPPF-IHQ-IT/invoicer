package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// SendInvoices sends eligible previously-created invoices for the fiscal year.
func SendInvoices(ctx context.Context, cfg *config.Config, plan *planner.Plan) (*Result, error) {
	qboClient, err := qbo.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	runID := time.Now().UTC().Format("2006-01-02T150405Z")

	for i := range plan.MemberPlans {
		mp := &plan.MemberPlans[i]
		if mp.Action != planner.ActionSend {
			continue
		}
		if mp.ExistingInvoice == nil {
			mp.Action = planner.ActionError
			mp.SkipDetail = "send action but no existing invoice reference"
			continue
		}

		// Re-fetch invoice before sending to get fresh state.
		fresh, err := qboClient.GetInvoice(ctx, mp.ExistingInvoice.ID)
		if err != nil {
			mp.Action = planner.ActionError
			mp.SkipDetail = fmt.Sprintf("re-fetching invoice %s: %v", mp.ExistingInvoice.ID, err)
			continue
		}
		if fresh.EmailStatus == "EmailSent" {
			mp.Action = planner.ActionSkip
			mp.SkipReason = "existing_invoice_already_sent"
			continue
		}
		if fresh.Balance <= 0 {
			mp.Action = planner.ActionSkip
			mp.SkipReason = "existing_invoice_paid"
			continue
		}
		if mp.QBOCustomer != nil && mp.QBOCustomer.Email == "" {
			mp.Action = planner.ActionSkip
			mp.SkipReason = "missing_email"
			continue
		}

		if err := qboClient.SendInvoice(ctx, mp.ExistingInvoice.ID); err != nil {
			mp.Action = planner.ActionError
			mp.SkipDetail = fmt.Sprintf("sending invoice %s: %v", mp.ExistingInvoice.ID, err)
			continue
		}
	}

	return &Result{Plan: plan, RunID: runID}, nil
}
