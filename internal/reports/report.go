package reports

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
)

type Mode string

const (
	ModePreview Mode = "preview"
	ModeRun     Mode = "run"
)

// Options controls how a report is written.
type Options struct {
	Mode    Mode
	Format  string // "table" or "json"
	OutFile string
	RunID   string
}

// Report is the JSON-serializable run/preview report.
type Report struct {
	RunID              string        `json:"run_id"`
	Mode               string        `json:"mode"`
	FiscalYear         string        `json:"fiscal_year"`
	AirtableFiscalYear string        `json:"airtable_fiscal_year"`
	PeriodStart        string        `json:"period_start"`
	PeriodEnd          string        `json:"period_end"`
	AsOf               string        `json:"as_of"`
	InvoiceDate        string        `json:"invoice_date"`
	DueDate            string        `json:"due_date"`
	Terms              string        `json:"terms"`
	Created            []MemberEntry `json:"created"`
	Updated            []MemberEntry `json:"updated"`
	SendCandidates     []MemberEntry `json:"send_candidates"`
	Sent               []MemberEntry `json:"sent"`
	VoidCandidates     []MemberEntry `json:"void_candidates"`
	NoChange           []MemberEntry `json:"no_change"`
	Skipped            []MemberEntry `json:"skipped"`
	Errors             []MemberEntry `json:"errors"`
}

// MemberEntry is a single member's result in the report.
type MemberEntry struct {
	ControlNumber string  `json:"control_number"`
	Name          string  `json:"name,omitempty"`
	Email         string  `json:"email"`
	QBOCustomerID string  `json:"qbo_customer_id,omitempty"`
	Action        string  `json:"action"`
	SkipReason    string  `json:"skip_reason,omitempty"`
	Detail        string  `json:"detail,omitempty"`
	Total         float64 `json:"total,omitempty"`
	InvoiceID     string  `json:"invoice_id,omitempty"`
	LineHash      string  `json:"line_hash,omitempty"`
}

// Write renders the plan as a report to stdout and optionally to a file.
func Write(plan *planner.Plan, opts Options) error {
	runID := opts.RunID
	if runID == "" {
		runID = time.Now().UTC().Format("2006-01-02T150405Z")
	}

	report := buildReport(plan, runID, string(opts.Mode))

	if opts.Format == "json" || opts.OutFile != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if opts.OutFile != "" {
			if err := os.MkdirAll(filepath.Dir(opts.OutFile), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(opts.OutFile, data, 0644); err != nil {
				return err
			}
		}
		if opts.Format == "json" {
			fmt.Println(string(data))
			return nil
		}
	}

	printTable(report)
	return nil
}

func buildReport(plan *planner.Plan, runID, mode string) Report {
	r := Report{
		RunID:              runID,
		Mode:               mode,
		FiscalYear:         plan.FY.Label,
		AirtableFiscalYear: plan.FY.AirtableLabel,
		PeriodStart:        plan.FY.PeriodStart.Format("2006-01-02"),
		PeriodEnd:          plan.FY.PeriodEnd.Format("2006-01-02"),
		AsOf:               plan.AsOf.Format("2006-01-02"),
		InvoiceDate:        plan.AsOf.Format("2006-01-02"),
		DueDate:            plan.AsOf.Format("2006-01-02"),
		Terms:              "Due on receipt",
	}

	for _, mp := range plan.MemberPlans {
		entry := MemberEntry{
			ControlNumber: mp.Member.ControlNumber,
			Email:         mp.Member.Email,
			Action:        string(mp.Action),
			SkipReason:    string(mp.SkipReason),
			Detail:        mp.SkipDetail,
		}
		if mp.QBOCustomer != nil {
			entry.QBOCustomerID = mp.QBOCustomer.ID
			entry.Name = mp.QBOCustomer.DisplayName
		}
		if mp.CalcResult != nil {
			entry.Total = mp.CalcResult.Total
			entry.LineHash = mp.CalcResult.LineHash
		}
		if mp.ExistingInvoice != nil {
			entry.InvoiceID = mp.ExistingInvoice.ID
		}

		switch mp.Action {
		case planner.ActionCreate:
			r.Created = append(r.Created, entry)
		case planner.ActionNoChange:
			r.NoChange = append(r.NoChange, entry)
		case planner.ActionSend:
			r.Sent = append(r.Sent, entry)
		case planner.ActionSkip:
			r.Skipped = append(r.Skipped, entry)
		case planner.ActionError:
			r.Errors = append(r.Errors, entry)
		}
	}
	return r
}
