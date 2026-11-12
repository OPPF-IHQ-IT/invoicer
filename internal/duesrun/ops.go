package duesrun

import (
	"context"
	"fmt"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
	"github.com/OPPF-IHQ-IT/invoicer/internal/runner"
)

// LiveOps runs against the real QBO and Airtable.
type LiveOps struct {
	Cfg  *config.Config
	FY   fiscal.Year
	AsOf time.Time
}

func (o *LiveOps) Gather(ctx context.Context) (*Snapshot, error) {
	return Gather(ctx, o.Cfg, o.FY, o.AsOf)
}

func (o *LiveOps) Create(ctx context.Context, plan *planner.Plan) (*planner.Plan, error) {
	res, err := runner.CreateInvoices(ctx, o.Cfg, plan)
	if err != nil {
		return nil, err
	}
	return res.Plan, nil
}

func (o *LiveOps) VerifyCreated(ctx context.Context, plan *planner.Plan) []string {
	client, err := qbo.NewClient(o.Cfg)
	if err != nil {
		return []string{err.Error()}
	}
	return VerifyCreated(ctx, client, plan)
}

func (o *LiveOps) SendPlan(ctx context.Context) (*planner.Plan, error) {
	return planner.Build(ctx, o.Cfg, o.FY, o.AsOf, true)
}

func (o *LiveOps) Send(ctx context.Context, plan *planner.Plan) (*planner.Plan, error) {
	res, err := runner.SendInvoices(ctx, o.Cfg, plan)
	if err != nil {
		return nil, err
	}
	return res.Plan, nil
}

func (o *LiveOps) VerifySent(ctx context.Context, plan *planner.Plan) []string {
	client, err := qbo.NewClient(o.Cfg)
	if err != nil {
		return []string{err.Error()}
	}
	return VerifySent(ctx, client, plan)
}

// DuesItems is the set of products that mark a document as dues.
func DuesItems(cfg *config.Config) map[string]bool {
	return map[string]bool{
		cfg.QBOItems.International: true, cfg.QBOItems.InternationalUndergrad: true,
		cfg.QBOItems.District: true, cfg.QBOItems.State: true,
		cfg.QBOItems.Local: true, cfg.QBOItems.LocalRetiree: true,
	}
}

// LoadStatus reads where a fiscal year stands in QBO.
func LoadStatus(ctx context.Context, cfg *config.Config, fy fiscal.Year) (*Status, error) {
	client, err := qbo.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	invoices, err := client.ListInvoicesBetween(ctx, fy.PeriodStart, fy.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("listing invoices: %w", err)
	}
	receipts, err := client.ListSalesReceiptsBetween(ctx, fy.PeriodStart, fy.PeriodEnd)
	if err != nil {
		return nil, fmt.Errorf("listing receipts: %w", err)
	}
	st := BuildStatus(invoices, receipts, DuesItems(cfg), fy)
	return &st, nil
}
