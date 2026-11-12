package duesrun

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// DocReader is the part of the QBO client verification needs.
type DocReader interface {
	GetInvoice(ctx context.Context, id string) (*qbo.Invoice, error)
	GetSalesReceipt(ctx context.Context, id string) (*qbo.SalesReceipt, error)
}

func lineKey(lines []qbo.InvoiceLine) map[string]int {
	keys := map[string]int{}
	for _, l := range lines {
		id := ""
		if l.SalesItemLineDetail != nil {
			id = l.SalesItemLineDetail.ItemRef.Value
		}
		keys[fmt.Sprintf("%s %.2f", id, l.Amount)]++
	}
	return keys
}

func sameLines(a, b []qbo.InvoiceLine) bool {
	ka, kb := lineKey(a), lineKey(b)
	if len(ka) != len(kb) {
		return false
	}
	for k, n := range ka {
		if kb[k] != n {
			return false
		}
	}
	return true
}

// VerifyCreated reads every document create just made back from QBO and
// compares it with the plan. It returns one line per problem.
func VerifyCreated(ctx context.Context, r DocReader, plan *planner.Plan) []string {
	var problems []string
	add := func(mp planner.MemberPlan, format string, args ...any) {
		problems = append(problems, fmt.Sprintf("%s %s: ", mp.Member.ControlNumber, mp.Member.Name)+fmt.Sprintf(format, args...))
	}
	for _, mp := range plan.MemberPlans {
		switch mp.Action {
		case planner.ActionError:
			add(mp, "%s", mp.SkipDetail)
			continue
		case planner.ActionCreate:
			if mp.ExistingInvoice == nil {
				add(mp, "no invoice came back from QBO")
				continue
			}
			inv, err := r.GetInvoice(ctx, mp.ExistingInvoice.ID)
			if err != nil {
				add(mp, "reading invoice back: %v", err)
				continue
			}
			if math.Abs(inv.TotalAmt-mp.CalcResult.Total) > 0.005 {
				add(mp, "QBO total $%.2f, planned $%.2f", inv.TotalAmt, mp.CalcResult.Total)
			}
			if !sameLines(inv.Line, mp.CalcResult.Lines) {
				add(mp, "QBO lines differ from the plan")
			}
			if inv.BillEmail == "" {
				add(mp, "invoice has no email address")
			}
		case planner.ActionCreateReceipt:
			if mp.ExistingReceipt == nil {
				add(mp, "no receipt came back from QBO")
				continue
			}
			sr, err := r.GetSalesReceipt(ctx, mp.ExistingReceipt.ID)
			if err != nil {
				add(mp, "reading receipt back: %v", err)
				continue
			}
			if math.Abs(sr.TotalAmt) > 0.005 {
				add(mp, "receipt total is $%.2f, not $0", sr.TotalAmt)
			}
			if !sameLines(sr.Line, mp.CalcResult.Lines) {
				add(mp, "QBO lines differ from the plan")
			}
		default:
			continue
		}
		// Create records partial failures here, such as a status update that
		// failed. Those need fixing by hand before any rerun.
		if mp.SkipDetail != "" {
			add(mp, "%s", mp.SkipDetail)
		}
	}
	return problems
}

// VerifySent confirms each sent document shows as emailed.
func VerifySent(ctx context.Context, r DocReader, plan *planner.Plan) []string {
	var problems []string
	add := func(mp planner.MemberPlan, format string, args ...any) {
		problems = append(problems, fmt.Sprintf("%s %s: ", mp.Member.ControlNumber, mp.Member.Name)+fmt.Sprintf(format, args...))
	}
	for _, mp := range plan.MemberPlans {
		switch mp.Action {
		case planner.ActionError:
			add(mp, "%s", mp.SkipDetail)
		case planner.ActionSend:
			inv, err := r.GetInvoice(ctx, mp.ExistingInvoice.ID)
			if err != nil {
				add(mp, "reading invoice back: %v", err)
			} else if inv.EmailStatus != "EmailSent" {
				add(mp, "invoice %s shows %q, not sent", inv.DocNumber, inv.EmailStatus)
			}
		case planner.ActionSendReceipt:
			sr, err := r.GetSalesReceipt(ctx, mp.ExistingReceipt.ID)
			if err != nil {
				add(mp, "reading receipt back: %v", err)
			} else if sr.EmailStatus != "EmailSent" {
				add(mp, "receipt %s shows %q, not sent", sr.DocNumber, sr.EmailStatus)
			}
		}
	}
	return problems
}

// Bounce is an invoice or receipt QBO couldn't deliver.
type Bounce struct {
	Customer, Document, Email, Reason string
	Amount                            float64
}

// Status is where a fiscal year's dues stand in QBO.
type Status struct {
	Invoices, Paid, Open, Unsent, Voided int
	Billed, Outstanding                  float64
	Receipts, ReceiptsUnsent             int
	Bounces                              []Bounce
}

// BuildStatus summarizes the fiscal year's dues documents. duesItems identifies
// dues documents, so facilities and event invoices don't count.
func BuildStatus(invoices []qbo.Invoice, receipts []qbo.SalesReceipt, duesItems map[string]bool, fy fiscal.Year) Status {
	var st Status
	for _, inv := range invoices {
		if !hasItem(inv.Line, duesItems) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(inv.PrivateNote), "voided") {
			st.Voided++
			continue
		}
		if inv.TotalAmt == 0 {
			continue
		}
		st.Invoices++
		st.Billed += inv.TotalAmt
		st.Outstanding += inv.Balance
		if inv.Balance > 0 {
			st.Open++
		} else {
			st.Paid++
		}
		if inv.EmailStatus != "EmailSent" {
			st.Unsent++
		}
		if inv.DeliveryError != "" {
			st.Bounces = append(st.Bounces, Bounce{inv.CustomerRef.Name, "invoice " + inv.DocNumber, inv.BillEmail, inv.DeliveryError, inv.TotalAmt})
		}
	}
	for _, sr := range receipts {
		if sr.TotalAmt != 0 || !hasItem(sr.Line, duesItems) {
			continue
		}
		st.Receipts++
		if sr.EmailStatus != "EmailSent" {
			st.ReceiptsUnsent++
		}
		if sr.DeliveryError != "" {
			st.Bounces = append(st.Bounces, Bounce{sr.CustomerRef.Name, "receipt " + sr.DocNumber, sr.BillEmail, sr.DeliveryError, 0})
		}
	}
	return st
}
