package planner

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/invoicecalc"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// Action describes what invoicer would do for a member.
type Action string

const (
	ActionCreate        Action = "CREATE"
	ActionNoChange      Action = "NO_CHANGE"
	ActionSend          Action = "SEND"
	ActionSkip          Action = "SKIP"
	ActionError         Action = "ERROR"
	// $0 members get a sales receipt instead of an invoice.
	ActionCreateReceipt Action = "CREATE_RECEIPT"
	ActionSendReceipt   Action = "SEND_RECEIPT"
	ActionVoidCandidate Action = "VOID_CANDIDATE"
)

// MemberPlan is the resolved plan for a single member.
type MemberPlan struct {
	Member          airtable.Member
	QBOCustomer     *qbo.Customer
	Action          Action
	SkipReason      invoicecalc.SkipReason
	SkipDetail      string
	CalcResult      *invoicecalc.CalculationResult
	ExistingInvoice *qbo.Invoice
	ExistingReceipt *qbo.SalesReceipt
	InvoiceDate     string
	DueDate         string
}

// Plan is the full set of member plans for a fiscal year run.
type Plan struct {
	FY            fiscal.Year
	AsOf          time.Time
	RunID         string
	SendMode      bool
	MemberPlans   []MemberPlan
}

// Build resolves Airtable members, QBO customers, and dues schedule into a Plan.
func Build(ctx context.Context, cfg *config.Config, fy fiscal.Year, asOf time.Time, sendMode bool) (*Plan, error) {
	atClient := airtable.NewClient(cfg.Airtable.APIKey, cfg.Airtable.BaseID)

	qboClient, err := qbo.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	// A create run moves members to Invoiced, so sending also has to look
	// at Invoiced members to find the invoices that are waiting to go out,
	// and at Active members, who may have a $0 receipt waiting.
	statuses := cfg.Airtable.InvoiceableStatuses
	if sendMode {
		statuses = append(slices.Clone(statuses), cfg.Airtable.StatusValues.Invoiced, cfg.Airtable.StatusValues.Active)
	}
	members, err := atClient.ListMembersWithStatus(ctx, &cfg.Airtable, statuses)
	if err != nil {
		return nil, fmt.Errorf("loading Airtable members: %w", err)
	}

	scheduleRows, err := atClient.ListDuesSchedule(ctx, &cfg.Airtable, fy.AirtableLabel)
	if err != nil {
		return nil, fmt.Errorf("loading dues schedule: %w", err)
	}
	if len(scheduleRows) == 0 {
		return nil, fmt.Errorf("no dues schedule rows found for %s or ALL; cannot proceed", fy.AirtableLabel)
	}

	memberCredits, err := atClient.ListMemberCredits(ctx, &cfg.Airtable)
	if err != nil {
		return nil, fmt.Errorf("loading member credits: %w", err)
	}

	customers, err := qboClient.ListCustomers(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading QBO customers: %w", err)
	}
	customerIndex := indexCustomers(customers)

	runID := asOf.UTC().Format("2006-01-02T150405Z")
	plan := &Plan{
		FY:       fy,
		AsOf:     asOf,
		RunID:    runID,
		SendMode: sendMode,
	}

	for _, member := range members {
		member = applyMSPExpiry(member, fy)
		mp := buildMemberPlan(ctx, cfg, atClient, qboClient, fy, asOf, member, scheduleRows, memberCredits[member.RecordID], customerIndex, sendMode)
		plan.MemberPlans = append(plan.MemberPlans, mp)
	}

	return plan, nil
}

// applyMSPExpiry ends the Recent MSP offset once the prepaid period is over.
// MSP Prepaid Through is inclusive: FY2027 means dues start in FY2028. A blank
// or unreadable value leaves the Recent MSP box in charge.
func applyMSPExpiry(m airtable.Member, fy fiscal.Year) airtable.Member {
	through, err := fiscal.Parse(strings.ToUpper(strings.TrimSpace(m.MSPPrepaidThrough)))
	if m.RecentMSP && err == nil && through.Number < fy.Number {
		m.RecentMSP = false
		m.MSPExpired = true
	}
	return m
}

func buildMemberPlan(
	ctx context.Context,
	cfg *config.Config,
	atClient *airtable.Client,
	qboClient *qbo.Client,
	fy fiscal.Year,
	asOf time.Time,
	member airtable.Member,
	scheduleRows []airtable.DuesScheduleRow,
	memberCredits []airtable.MemberCredit,
	customerIndex map[string]*qbo.Customer,
	sendMode bool,
) MemberPlan {
	mp := MemberPlan{Member: member}

	// Resolve QBO customer.
	customer, skipReason, detail := resolveCustomer(member, customerIndex, cfg.Invoice.AllowNameFallbackInRun)
	// An Active member can only have a $0 receipt to send. Anything else about
	// them, such as a missing customer, isn't this run's concern.
	active := sendMode && member.Status == cfg.Airtable.StatusValues.Active
	if active && skipReason != invoicecalc.SkipReasonNone {
		mp.Action = ActionNoChange
		return mp
	}
	if skipReason != invoicecalc.SkipReasonNone {
		mp.Action = ActionSkip
		mp.SkipReason = skipReason
		mp.SkipDetail = detail
		return mp
	}
	mp.QBOCustomer = customer

	if active {
		planReceiptSend(ctx, qboClient, fy, &mp)
		return mp
	}

	// Load poll worker credits (sum all available across all FYs, FIFO).
	credits, err := atClient.ListPollWorkerCredits(ctx, &cfg.Airtable, member.RecordID)
	if err != nil {
		mp.Action = ActionError
		mp.SkipDetail = fmt.Sprintf("loading poll worker credits: %v", err)
		return mp
	}
	var totalCredit float64
	for _, c := range credits {
		totalCredit += c.CreditsAvailable
	}

	// Calculate expected invoice.
	calcResult := invoicecalc.Calculate(invoicecalc.CalculationInput{
		FY:               fy,
		AsOf:             asOf,
		Member:           member,
		PollWorkerCredit: totalCredit,
		MemberCredits:    memberCredits,
		Schedule:         scheduleRows,
		Items:            cfg.QBOItems,
	})
	if calcResult.SkipReason != invoicecalc.SkipReasonNone {
		mp.Action = ActionSkip
		mp.SkipReason = calcResult.SkipReason
		mp.SkipDetail = calcResult.SkipDetail
		return mp
	}
	mp.CalcResult = &calcResult
	mp.InvoiceDate = asOf.Format("2006-01-02")
	mp.DueDate = asOf.Format("2006-01-02")

	// Check for existing invoices.
	existing, err := qboClient.QueryInvoices(ctx, customer.ID, fy.PeriodStart, fy.PeriodEnd)
	if err != nil {
		mp.Action = ActionError
		mp.SkipDetail = fmt.Sprintf("querying QBO invoices: %v", err)
		return mp
	}

	var matched []*qbo.Invoice
	if sendMode {
		// Creating an invoice marks the credits it used as spent, so the
		// line hash recalculated now may no longer match the invoice that
		// was created. Find the member's invoice by record ID instead.
		matched = matchMemberInvoice(existing, fy, member.RecordID)
	}
	if len(matched) == 0 {
		matched = matchExistingInvoice(existing, calcResult.LineHash, fy)
	}
	switch len(matched) {
	case 0:
		if sendMode {
			// A $0 member whose status update failed still has a receipt to send.
			planReceiptSend(ctx, qboClient, fy, &mp)
			if mp.Action == ActionNoChange && mp.ExistingReceipt == nil {
				mp.Action = ActionSkip
				mp.SkipReason = "no_invoice_to_send"
				mp.SkipDetail = fmt.Sprintf("no %s invoice found for customer %s; run --create-only first", fy.Label, customer.ID)
			}
			return mp
		}
		if calcResult.IsZeroTotal {
			planReceiptCreate(ctx, qboClient, fy, &mp)
			return mp
		}
		mp.Action = ActionCreate
	case 1:
		mp.ExistingInvoice = matched[0]
		if sendMode {
			if canSend(matched[0]) {
				mp.Action = ActionSend
			} else {
				mp.Action = ActionSkip
				mp.SkipReason = skippedSendReason(matched[0])
			}
		} else {
			mp.Action = ActionNoChange
		}
	default:
		mp.Action = ActionSkip
		mp.SkipReason = "duplicate_invoice_conflict"
		mp.SkipDetail = fmt.Sprintf("%d matching invoices found for customer %s in %s", len(matched), customer.ID, fy.Label)
	}

	return mp
}

// memberReceipts returns the member's invoicer-created receipts for the fiscal year.
func memberReceipts(ctx context.Context, qboClient *qbo.Client, fy fiscal.Year, mp *MemberPlan) ([]*qbo.SalesReceipt, error) {
	receipts, err := qboClient.QuerySalesReceipts(ctx, mp.QBOCustomer.ID, fy.PeriodStart, fy.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("querying QBO sales receipts: %w", err)
	}
	return matchMemberReceipt(receipts, fy, mp.Member.RecordID), nil
}

// planReceiptCreate decides whether a $0 member needs a sales receipt.
func planReceiptCreate(ctx context.Context, qboClient *qbo.Client, fy fiscal.Year, mp *MemberPlan) {
	matched, err := memberReceipts(ctx, qboClient, fy, mp)
	if err != nil {
		mp.Action = ActionError
		mp.SkipDetail = err.Error()
		return
	}
	mp.Action, mp.SkipReason, mp.SkipDetail = receiptCreateAction(matched)
	if len(matched) == 1 {
		mp.ExistingReceipt = matched[0]
	}
}

func receiptCreateAction(matched []*qbo.SalesReceipt) (Action, invoicecalc.SkipReason, string) {
	switch len(matched) {
	case 0:
		return ActionCreateReceipt, invoicecalc.SkipReasonNone, ""
	case 1:
		return ActionNoChange, invoicecalc.SkipReasonNone, ""
	default:
		return ActionSkip, "duplicate_receipt_conflict", fmt.Sprintf("%d sales receipts found for this fiscal year", len(matched))
	}
}

// planReceiptSend decides whether a member has a sales receipt to email.
func planReceiptSend(ctx context.Context, qboClient *qbo.Client, fy fiscal.Year, mp *MemberPlan) {
	matched, err := memberReceipts(ctx, qboClient, fy, mp)
	if err != nil {
		mp.Action = ActionError
		mp.SkipDetail = err.Error()
		return
	}
	mp.Action, mp.SkipReason, mp.SkipDetail = receiptSendAction(matched)
	if len(matched) == 1 {
		mp.ExistingReceipt = matched[0]
	}
}

func receiptSendAction(matched []*qbo.SalesReceipt) (Action, invoicecalc.SkipReason, string) {
	switch {
	case len(matched) == 0:
		return ActionNoChange, invoicecalc.SkipReasonNone, ""
	case len(matched) > 1:
		return ActionSkip, "duplicate_receipt_conflict", fmt.Sprintf("%d sales receipts found for this fiscal year", len(matched))
	case matched[0].EmailStatus == "EmailSent":
		return ActionNoChange, invoicecalc.SkipReasonNone, ""
	default:
		return ActionSendReceipt, invoicecalc.SkipReasonNone, ""
	}
}

func canSend(inv *qbo.Invoice) bool {
	if inv.EmailStatus == "EmailSent" {
		return false
	}
	if inv.Balance <= 0 {
		return false
	}
	return true
}

func skippedSendReason(inv *qbo.Invoice) invoicecalc.SkipReason {
	if inv.EmailStatus == "EmailSent" {
		return "existing_invoice_already_sent"
	}
	if inv.Balance <= 0 {
		return "existing_invoice_paid"
	}
	return "existing_invoice_already_current"
}

type customerLookup struct {
	byID    map[string]*qbo.Customer
	byEmail map[string][]*qbo.Customer
	byName  map[string][]*qbo.Customer
}

func indexCustomers(customers []qbo.Customer) map[string]*qbo.Customer {
	m := make(map[string]*qbo.Customer, len(customers))
	for i := range customers {
		m[customers[i].ID] = &customers[i]
	}
	return m
}

func resolveCustomer(member airtable.Member, index map[string]*qbo.Customer, allowNameFallback bool) (*qbo.Customer, invoicecalc.SkipReason, string) {
	// 1. QBO Customer ID from Airtable (preferred).
	if member.QBOCustomerID != "" {
		c, ok := index[member.QBOCustomerID]
		if !ok {
			return nil, "missing_qbo_customer_id", fmt.Sprintf("QBO customer %q not found", member.QBOCustomerID)
		}
		return c, invoicecalc.SkipReasonNone, ""
	}

	if member.Email == "" {
		return nil, "missing_email", "member has no email and no QBO Customer ID"
	}

	// 2. Email match (case-insensitive).
	emailNorm := strings.ToLower(strings.TrimSpace(member.Email))
	var emailMatches []*qbo.Customer
	for _, c := range index {
		if strings.ToLower(strings.TrimSpace(c.Email)) == emailNorm {
			emailMatches = append(emailMatches, c)
		}
	}
	if len(emailMatches) == 1 {
		return emailMatches[0], invoicecalc.SkipReasonNone, ""
	}
	if len(emailMatches) > 1 {
		return nil, "customer_resolution_ambiguous", fmt.Sprintf("multiple QBO customers match email %q", member.Email)
	}

	if !allowNameFallback {
		return nil, "name_fallback_disabled", "no QBO customer matched by email and name fallback is disabled"
	}

	// 3. Name fallback (trimmed, collapsed whitespace).
	nameNorm := normalizeName(member.ControlNumber) // use control # as name search is risky
	_ = nameNorm
	return nil, "customer_resolution_failed", "no QBO customer found for member"
}

func normalizeName(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
