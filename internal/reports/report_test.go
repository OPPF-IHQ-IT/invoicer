package reports

import (
	"testing"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/invoicecalc"
	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// After create, credits are spent, so a recalculated total overstates what the
// invoice says. Send reports must show the invoice's own total.
func TestSendReportUsesDocumentTotals(t *testing.T) {
	fy, err := fiscal.Parse("FY2027")
	if err != nil {
		t.Fatal(err)
	}
	plan := &planner.Plan{FY: fy, MemberPlans: []planner.MemberPlan{
		{
			Member:          airtable.Member{ControlNumber: "1"},
			Action:          planner.ActionSend,
			CalcResult:      &invoicecalc.CalculationResult{Total: 408},
			ExistingInvoice: &qbo.Invoice{ID: "i", TotalAmt: 358},
		},
		{
			Member:          airtable.Member{ControlNumber: "2"},
			Action:          planner.ActionSendReceipt,
			CalcResult:      &invoicecalc.CalculationResult{Total: 148},
			ExistingReceipt: &qbo.SalesReceipt{ID: "r", TotalAmt: 0},
		},
	}}
	r := buildReport(plan, "run", "preview")
	if len(r.Sent) != 1 || r.Sent[0].Total != 358 {
		t.Fatalf("sent = %+v", r.Sent)
	}
	if len(r.Receipts) != 1 || r.Receipts[0].Total != 0 || r.Receipts[0].ReceiptID != "r" {
		t.Fatalf("receipts = %+v", r.Receipts)
	}
}
