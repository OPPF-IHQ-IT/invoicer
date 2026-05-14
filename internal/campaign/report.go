package campaign

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// ReportOptions controls the optional output files.
type ReportOptions struct {
	MatchedOut   string
	UnmatchedOut string
	SkippedOut   string
	JSONOut      string
}

// Summary is what we print to stdout after reconciliation (and again after a
// run, with outcome counts populated).
type Summary struct {
	CampaignName  string
	RunID         string
	DryRun        bool
	Matched       int
	Unmatched     int
	Skipped       int
	Superseded    int
	Created       int
	Sent          int
	CreatedNotSent int
	CreateFailed  int
	SendFailed    int
	CustomerCreated int
}

// WriteReconciliationCSVs writes the matched / unmatched / skipped CSVs when
// the corresponding paths are set.
func WriteReconciliationCSVs(res *Result, superseded []Row, opts ReportOptions) error {
	if opts.MatchedOut != "" {
		rows := [][]string{{"control_number", "name", "email", "qbo_customer_id", "amount", "amount_source", "needs_qbo_create"}}
		for _, m := range res.Matched {
			rows = append(rows, []string{
				m.Member.ControlNumber,
				m.Member.Name,
				m.Member.Email,
				m.Member.QBOCustomerID,
				strconv.FormatFloat(m.Amount, 'f', 2, 64),
				string(m.AmountSource),
				strconv.FormatBool(m.NeedsQBOCreate),
			})
		}
		if err := writeCSV(opts.MatchedOut, rows); err != nil {
			return fmt.Errorf("writing matched CSV: %w", err)
		}
	}

	if opts.UnmatchedOut != "" {
		rows := [][]string{{"control_number", "name", "email", "reason"}}
		for _, u := range res.Unmatched {
			rows = append(rows, []string{
				u.Row.ControlNumber,
				u.Row.FullName,
				u.Row.Email,
				u.Reason,
			})
		}
		if err := writeCSV(opts.UnmatchedOut, rows); err != nil {
			return fmt.Errorf("writing unmatched CSV: %w", err)
		}
	}

	if opts.SkippedOut != "" {
		rows := [][]string{{"control_number", "name", "email", "reason"}}
		for _, s := range res.Skipped {
			rows = append(rows, []string{s.Row.ControlNumber, s.Row.FullName, s.Row.Email, s.Reason})
		}
		for _, r := range superseded {
			rows = append(rows, []string{r.ControlNumber, r.FullName, r.Email, "duplicate_superseded"})
		}
		if err := writeCSV(opts.SkippedOut, rows); err != nil {
			return fmt.Errorf("writing skipped CSV: %w", err)
		}
	}

	return nil
}

// WriteJSONReport writes a JSON snapshot of the whole campaign run.
func WriteJSONReport(path string, payload any) error {
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating report dir: %w", err)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating JSON report: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// PrintSummary prints a human-readable summary to w.
func PrintSummary(w io.Writer, s Summary, hasRun bool) {
	fmt.Fprintln(w, "Campaign reconciliation summary:")
	if s.CampaignName != "" {
		fmt.Fprintf(w, "  Campaign:                %s\n", s.CampaignName)
	}
	fmt.Fprintf(w, "  Matched (will invoice):  %d\n", s.Matched)
	fmt.Fprintf(w, "  Unmatched:               %d\n", s.Unmatched)
	fmt.Fprintf(w, "  Skipped:                 %d\n", s.Skipped)
	if s.Superseded > 0 {
		fmt.Fprintf(w, "  Superseded duplicates:   %d\n", s.Superseded)
	}
	if !hasRun {
		fmt.Fprintln(w, "\nDry run — no invoices created. Re-run with --no-dry-run --yes to create+send.")
		return
	}
	fmt.Fprintln(w, "\nRun outcomes:")
	fmt.Fprintf(w, "  Run ID:                  %s\n", s.RunID)
	fmt.Fprintf(w, "  QBO customers created:   %d\n", s.CustomerCreated)
	fmt.Fprintf(w, "  Invoices created:        %d\n", s.Created)
	fmt.Fprintf(w, "  Invoices sent:           %d\n", s.Sent)
	fmt.Fprintf(w, "  Created (not sent):      %d\n", s.CreatedNotSent)
	fmt.Fprintf(w, "  Create failures:         %d\n", s.CreateFailed)
	fmt.Fprintf(w, "  Send failures:           %d\n", s.SendFailed)
}

// JSONReport is the on-disk structure for --out.
type JSONReport struct {
	CampaignName string         `json:"campaign_name,omitempty"`
	RunID        string         `json:"run_id,omitempty"`
	DryRun       bool           `json:"dry_run"`
	ItemID       string         `json:"item_id,omitempty"`
	Counts       Summary        `json:"counts"`
	Matched      []MatchedRow   `json:"matched,omitempty"`
	Unmatched    []UnmatchedRow `json:"unmatched,omitempty"`
	Skipped      []SkippedRow   `json:"skipped,omitempty"`
	Superseded   []Row          `json:"superseded,omitempty"`
	Outcomes     []PerRowOutcome `json:"outcomes,omitempty"`
}

// CountsFromRun derives a Summary from reconciliation result + run result.
// runResult may be nil for dry-run.
func CountsFromRun(res *Result, superseded []Row, runResult *RunResult, campaignName string) Summary {
	s := Summary{
		CampaignName: campaignName,
		Matched:      len(res.Matched),
		Unmatched:    len(res.Unmatched),
		Skipped:      len(res.Skipped),
		Superseded:   len(superseded),
	}
	if runResult == nil {
		return s
	}
	s.RunID = runResult.RunID
	for _, o := range runResult.Outcomes {
		switch o.Status {
		case "invoiced_and_sent":
			s.Created++
			s.Sent++
		case "created_not_sent":
			s.Created++
			s.CreatedNotSent++
		case "created_send_failed":
			s.Created++
			s.SendFailed++
		case "create_failed":
			s.CreateFailed++
		case "customer_create_failed":
			s.CreateFailed++
		}
	}
	// QBO customer creation count: any matched row that needed creation AND the
	// outcome doesn't carry a customer_create_failed status.
	for i, m := range res.Matched {
		if !m.NeedsQBOCreate {
			continue
		}
		if i < len(runResult.Outcomes) && runResult.Outcomes[i].Status == "customer_create_failed" {
			continue
		}
		s.CustomerCreated++
	}
	return s
}

func writeCSV(path string, rows [][]string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
