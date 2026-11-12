package duesrun

import (
	"context"

	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// DryRunOps walks the whole flow with real data but changes nothing. It holds
// only a read function, so it has no way to reach QBO or Airtable writes:
// create and send are simulated in memory.
type DryRunOps struct {
	GatherFn func(ctx context.Context) (*Snapshot, error)
	pending  *planner.Plan
}

func (o *DryRunOps) Gather(ctx context.Context) (*Snapshot, error) { return o.GatherFn(ctx) }

// Create pretends to create the documents, so the send stage has something to
// show. The result stays in memory.
func (o *DryRunOps) Create(_ context.Context, plan *planner.Plan) (*planner.Plan, error) {
	sim := *plan
	sim.MemberPlans = make([]planner.MemberPlan, len(plan.MemberPlans))
	copy(sim.MemberPlans, plan.MemberPlans)
	for i := range sim.MemberPlans {
		mp := &sim.MemberPlans[i]
		switch mp.Action {
		case planner.ActionCreate:
			mp.Action = planner.ActionSend
			mp.ExistingInvoice = &qbo.Invoice{TotalAmt: mp.CalcResult.Total}
		case planner.ActionCreateReceipt:
			mp.Action = planner.ActionSendReceipt
			mp.ExistingReceipt = &qbo.SalesReceipt{}
		default:
			mp.Action = planner.ActionNoChange
		}
	}
	o.pending = &sim
	return plan, nil
}

func (o *DryRunOps) VerifyCreated(context.Context, *planner.Plan) []string { return nil }

func (o *DryRunOps) SendPlan(context.Context) (*planner.Plan, error) {
	if o.pending == nil {
		return &planner.Plan{}, nil
	}
	return o.pending, nil
}

func (o *DryRunOps) Send(_ context.Context, plan *planner.Plan) (*planner.Plan, error) {
	o.pending = nil
	return plan, nil
}

func (o *DryRunOps) VerifySent(context.Context, *planner.Plan) []string { return nil }
