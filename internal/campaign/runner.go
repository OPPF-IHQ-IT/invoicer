package campaign

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// RunOptions controls the create+send execution phase.
type RunOptions struct {
	ItemID       string
	CampaignName string
	CustomerMemo string // overrides cfg.Invoice.CustomerMemo when non-empty
}

// PerRowOutcome captures what happened for a single matched row during Execute.
type PerRowOutcome struct {
	ControlNumber string
	Name          string
	Email         string
	Amount        float64
	// Status is one of: "invoiced_and_sent", "created_not_sent",
	// "created_send_failed", "create_failed", "customer_create_failed".
	Status      string
	InvoiceID   string
	DocNumber   string
	QBOCustomer string // QBO customer ID (post-creation if applicable)
	Error       string
}

// RunResult aggregates per-row outcomes.
type RunResult struct {
	RunID    string
	Outcomes []PerRowOutcome
}

// Execute creates and sends QBO invoices for each matched row. Failures on
// individual rows are recorded but do not abort the run. The Airtable QBO
// Customer ID is written back when a missing customer is created.
func Execute(ctx context.Context, cfg *config.Config, matched []MatchedRow, opts RunOptions) (*RunResult, error) {
	qboClient, err := qbo.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	atClient := airtable.NewClient(cfg.Airtable.APIKey, cfg.Airtable.BaseID)

	runID := time.Now().UTC().Format("2006-01-02T150405Z")
	txnDate := time.Now().UTC().Format("2006-01-02")

	memo := opts.CustomerMemo
	if memo == "" {
		memo = cfg.Invoice.CustomerMemo
	}
	description := opts.CampaignName
	if description == "" {
		description = "Campaign contribution"
	}

	customers, err := qboClient.ListCustomers(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading QBO customers: %w", err)
	}
	customerByID := make(map[string]*qbo.Customer, len(customers))
	for i := range customers {
		customerByID[customers[i].ID] = &customers[i]
	}

	nextDocNumber, err := qboClient.NextInvoiceNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching next invoice number: %w", err)
	}

	result := &RunResult{RunID: runID}

	for _, m := range matched {
		out := PerRowOutcome{
			ControlNumber: m.Member.ControlNumber,
			Name:          m.Member.Name,
			Email:         m.Member.Email,
			Amount:        m.Amount,
			QBOCustomer:   m.Member.QBOCustomerID,
		}

		customerID := m.Member.QBOCustomerID
		customerEmail := ""
		if cust, ok := customerByID[customerID]; ok {
			customerEmail = cust.Email
		}
		if customerEmail == "" {
			customerEmail = m.Member.Email
		}

		if m.NeedsQBOCreate {
			cust, err := qboClient.CreateCustomer(ctx, m.CreationName, m.CreationEmail, m.Member.ControlNumber)
			if err != nil {
				out.Status = "customer_create_failed"
				out.Error = err.Error()
				fmt.Fprintf(os.Stderr, "campaign: creating QBO customer for %s: %v\n", m.Member.ControlNumber, err)
				result.Outcomes = append(result.Outcomes, out)
				continue
			}
			customerID = cust.ID
			customerEmail = cust.Email
			out.QBOCustomer = cust.ID
			out.Name = cust.DisplayName
			out.Email = cust.Email
			fmt.Printf("  Created QBO customer for %s (%s) → ID %s\n", m.Member.ControlNumber, m.CreationEmail, cust.ID)

			if err := atClient.UpdateMemberQBOCustomerID(ctx, &cfg.Airtable, m.Member.RecordID, cust.ID); err != nil {
				fmt.Fprintf(os.Stderr, "campaign: writing back QBO Customer ID for %s: %v\n", m.Member.ControlNumber, err)
			}
		}

		privateNote := buildPrivateNote(opts.CampaignName, m.Member.ControlNumber, runID)

		inv, err := qboClient.CreateInvoice(ctx, qbo.InvoiceCreateRequest{
			CustomerRef:  qbo.CustomerRef{Value: customerID},
			DocNumber:    nextDocNumber,
			TxnDate:      txnDate,
			DueDate:      txnDate, // due on receipt
			PrivateNote:  privateNote,
			CustomerMemo: memo,
			BillEmail:    customerEmail,
			SalesTermID:  cfg.Invoice.SalesTermID,
			Line: []qbo.InvoiceLine{
				{
					Amount:      m.Amount,
					DetailType:  "SalesItemLineDetail",
					Description: description,
					SalesItemLineDetail: &qbo.SalesItemLineDetail{
						ItemRef:   qbo.ItemRef{Value: opts.ItemID},
						Qty:       1,
						UnitPrice: m.Amount,
					},
				},
			},
		})
		if err != nil {
			out.Status = "create_failed"
			out.Error = err.Error()
			fmt.Fprintf(os.Stderr, "campaign: creating invoice for %s: %v\n", m.Member.ControlNumber, err)
			result.Outcomes = append(result.Outcomes, out)
			continue
		}
		out.InvoiceID = inv.ID
		out.DocNumber = inv.DocNumber
		nextDocNumber = qbo.IncrementDocNumber(nextDocNumber)

		if customerEmail == "" {
			out.Status = "created_not_sent"
			out.Error = "QBO customer has no email address"
			fmt.Fprintf(os.Stderr, "campaign: invoice %s created for %s but customer has no email; not sending\n", inv.ID, m.Member.ControlNumber)
			result.Outcomes = append(result.Outcomes, out)
			continue
		}

		if err := qboClient.SendInvoice(ctx, inv.ID); err != nil {
			out.Status = "created_send_failed"
			out.Error = err.Error()
			fmt.Fprintf(os.Stderr, "campaign: sending invoice %s for %s: %v\n", inv.ID, m.Member.ControlNumber, err)
			result.Outcomes = append(result.Outcomes, out)
			continue
		}

		out.Status = "invoiced_and_sent"
		result.Outcomes = append(result.Outcomes, out)
	}

	return result, nil
}

func buildPrivateNote(campaignName, controlNumber, runID string) string {
	name := campaignName
	if name == "" {
		name = "campaign"
	}
	return fmt.Sprintf("invoicer campaign=%s control_number=%s run_id=%s", name, controlNumber, runID)
}

// VerifyItemID confirms an item ID exists in QBO before reconciling. Returns a
// helpful error listing nothing — operators can run `invoicer qbo doctor` to
// list items.
func VerifyItemID(ctx context.Context, cfg *config.Config, itemID string) error {
	qboClient, err := qbo.NewClient(cfg)
	if err != nil {
		return err
	}
	items, err := qboClient.ListItems(ctx)
	if err != nil {
		return fmt.Errorf("listing QBO items: %w", err)
	}
	for _, it := range items {
		if it.ID == itemID {
			return nil
		}
	}
	return fmt.Errorf("QBO item %q not found (run 'invoicer qbo doctor' to list active items)", itemID)
}
