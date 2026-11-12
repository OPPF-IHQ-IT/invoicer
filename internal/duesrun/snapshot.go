// Package duesrun implements the guided annual dues run: pre-flight checks, the
// review file, verification against QBO, and the staged flow that ties them
// together. Planning and document creation stay in the planner and runner.
package duesrun

import (
	"context"
	"fmt"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// Snapshot is everything the checks look at. It's gathered once, so checks are
// plain functions of data and can be tested without QBO or Airtable.
type Snapshot struct {
	FY      fiscal.Year
	AsOf    time.Time
	Env     string
	Company string

	Plan *planner.Plan

	Schedule      []airtable.DuesScheduleRow
	PriorSchedule []airtable.DuesScheduleRow
	PollRows      []airtable.CreditRow
	CreditRows    []airtable.CreditRow
	MemberCredits map[string][]airtable.MemberCredit

	Customers []qbo.Customer
	Items     []qbo.Item
	Terms     []qbo.Term

	Invoices       []qbo.Invoice      // dated in this fiscal year
	Receipts       []qbo.SalesReceipt // dated in this fiscal year
	PriorInvoices  []qbo.Invoice      // from the prior fiscal year
	PriorReceipts  []qbo.SalesReceipt // from the prior fiscal year
	RecentReceipts []qbo.SalesReceipt // the 90 days before the fiscal year
	RecentPayments []qbo.Payment      // the 90 days before the as-of date
}

// Gather loads a Snapshot. It only reads, apart from QBO saving a refreshed token.
func Gather(ctx context.Context, cfg *config.Config, fy fiscal.Year, asOf time.Time) (*Snapshot, error) {
	prior, err := fiscal.Parse(fmt.Sprintf("%d", fy.Number-1))
	if err != nil {
		return nil, err
	}
	client, err := qbo.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	at := airtable.NewClient(cfg.Airtable.APIKey, cfg.Airtable.BaseID)

	s := &Snapshot{FY: fy, AsOf: asOf, Env: cfg.QBO.Environment}
	if s.Company, err = client.CompanyName(ctx); err != nil {
		return nil, fmt.Errorf("connecting to QBO (%s): %w\nLog in with: invoicer auth login --env %s", cfg.QBO.Environment, err, cfg.QBO.Environment)
	}

	if s.Plan, err = planner.Build(ctx, cfg, fy, asOf, false); err != nil {
		return nil, fmt.Errorf("building plan: %w", err)
	}
	if s.Schedule, err = at.ListDuesSchedule(ctx, &cfg.Airtable, fy.AirtableLabel); err != nil {
		return nil, err
	}
	if s.PriorSchedule, err = at.ListDuesSchedule(ctx, &cfg.Airtable, prior.AirtableLabel); err != nil {
		return nil, err
	}
	if s.PollRows, err = at.ListPollWorkerCreditRows(ctx, &cfg.Airtable); err != nil {
		return nil, err
	}
	if s.CreditRows, err = at.ListMemberCreditRows(ctx, &cfg.Airtable); err != nil {
		return nil, err
	}
	if s.MemberCredits, err = at.ListMemberCredits(ctx, &cfg.Airtable); err != nil {
		return nil, err
	}

	if s.Customers, err = client.ListCustomers(ctx); err != nil {
		return nil, err
	}
	if s.Items, err = client.ListItems(ctx); err != nil {
		return nil, err
	}
	if s.Terms, err = client.ListTerms(ctx); err != nil {
		return nil, err
	}
	if s.Invoices, err = client.ListInvoicesBetween(ctx, fy.PeriodStart, fy.PeriodEnd); err != nil {
		return nil, err
	}
	if s.Receipts, err = client.ListSalesReceiptsBetween(ctx, fy.PeriodStart, fy.PeriodEnd); err != nil {
		return nil, err
	}
	if s.PriorInvoices, err = client.ListInvoicesBetween(ctx, prior.PeriodStart, prior.PeriodEnd); err != nil {
		return nil, err
	}
	if s.PriorReceipts, err = client.ListSalesReceiptsBetween(ctx, prior.PeriodStart, prior.PeriodEnd); err != nil {
		return nil, err
	}
	lookback := fy.PeriodStart.AddDate(0, 0, -90)
	if s.RecentReceipts, err = client.ListSalesReceiptsBetween(ctx, lookback, fy.PeriodEnd); err != nil {
		return nil, err
	}
	if s.RecentPayments, err = client.ListPaymentsBetween(ctx, asOf.AddDate(0, 0, -90), asOf); err != nil {
		return nil, err
	}
	return s, nil
}
