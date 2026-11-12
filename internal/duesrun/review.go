package duesrun

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
)

// ReviewState is what the KRS approved. Create refuses to run unless the plan
// still matches it.
type ReviewState struct {
	FiscalYear  string            `json:"fiscal_year"`
	InvoiceDate string            `json:"invoice_date"`
	Fingerprint string            `json:"fingerprint"`
	Approved    bool              `json:"approved"`
	ApprovedAt  string            `json:"approved_at,omitempty"`
	Members     map[string]string `json:"members"` // control number -> what will be created
}

// memberKeys describes each document to be created, so a changed plan can show
// exactly who changed.
func memberKeys(plan *planner.Plan) map[string]string {
	keys := map[string]string{}
	for _, mp := range plan.MemberPlans {
		if !creates(mp) {
			continue
		}
		email := ""
		if mp.QBOCustomer != nil {
			email = mp.QBOCustomer.Email
		}
		keys[mp.Member.ControlNumber] = fmt.Sprintf("%s $%.2f %s %s", mp.Action, mp.CalcResult.Total, mp.CalcResult.LineHash, email)
	}
	return keys
}

// Fingerprint identifies a plan: same members, documents, lines, and addresses.
func Fingerprint(plan *planner.Plan) string {
	keys := memberKeys(plan)
	controls := make([]string, 0, len(keys))
	for c := range keys {
		controls = append(controls, c)
	}
	sort.Strings(controls)
	h := sha256.New()
	fmt.Fprintf(h, "%s\n", plan.AsOf.Format("2006-01-02"))
	for _, c := range controls {
		fmt.Fprintf(h, "%s=%s\n", c, keys[c])
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// NewReviewState captures a plan for approval.
func NewReviewState(plan *planner.Plan) *ReviewState {
	return &ReviewState{
		FiscalYear:  plan.FY.Label,
		InvoiceDate: plan.AsOf.Format("2006-01-02"),
		Fingerprint: Fingerprint(plan),
		Members:     memberKeys(plan),
	}
}

// DiffPlan lists what changed since a plan was reviewed.
func DiffPlan(reviewed *ReviewState, plan *planner.Plan) []string {
	now := memberKeys(plan)
	names := map[string]string{}
	for _, mp := range plan.MemberPlans {
		names[mp.Member.ControlNumber] = mp.Member.Name
	}
	var out []string
	for c, was := range reviewed.Members {
		is, ok := now[c]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s %s: no longer being created (was %s)", c, names[c], was))
		case is != was:
			out = append(out, fmt.Sprintf("%s %s: was %s, now %s", c, names[c], was, is))
		}
	}
	for c, is := range now {
		if _, ok := reviewed.Members[c]; !ok {
			out = append(out, fmt.Sprintf("%s %s: new since review (%s)", c, names[c], is))
		}
	}
	sort.Strings(out)
	return out
}

// RunDir is where a fiscal year's review files live. They hold member data, so
// they stay in the git-ignored report directory, readable only by this user.
func RunDir(reportDir, fyLabel string) (string, error) {
	if reportDir == "" {
		reportDir = "./.invoicer-runs"
	}
	dir := filepath.Join(reportDir, fyLabel)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// LoadReview returns the saved review state, or nil if there isn't one.
func LoadReview(dir string) (*ReviewState, error) {
	data, err := os.ReadFile(filepath.Join(dir, "review-state.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var st ReviewState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// SaveReview writes the review state.
func SaveReview(dir string, st *ReviewState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "review-state.json"), data, 0o600)
}

// WriteReviewFile writes the review spreadsheet and returns its path.
func WriteReviewFile(dir string, s *Snapshot, findings []Finding) (string, error) {
	path := filepath.Join(dir, "review.csv")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := writeReview(f, s, findings); err != nil {
		return "", err
	}
	return path, nil
}

func writeReview(out io.Writer, s *Snapshot, findings []Finding) error {
	w := csv.NewWriter(out)
	last := lastYearDoc(s)

	byMember := map[string][]string{}
	var general []Finding
	for _, f := range findings {
		if f.Control == "" {
			general = append(general, f)
			continue
		}
		byMember[f.Control] = append(byMember[f.Control], fmt.Sprintf("[%s] %s", f.Level, f.Message))
	}

	summary := Summarize(s.Plan)
	head := [][]string{
		{fmt.Sprintf("%s dues review", s.FY.Label), "", fmt.Sprintf("Invoice date %s", s.AsOf.Format("2006-01-02"))},
		{"Company", s.Company, "Environment", s.Env},
		{"Invoices", fmt.Sprint(summary.Invoices), "Total billed", fmt.Sprintf("%.2f", summary.Billed)},
		{"$0 receipts", fmt.Sprint(summary.Receipts), "Skipped", fmt.Sprint(summary.Skipped)},
		{"Blockers", fmt.Sprint(CountByLevel(findings, Blocker)), "Warnings", fmt.Sprint(CountByLevel(findings, Warning))},
	}
	for _, f := range general {
		head = append(head, []string{string(f.Level), cell(f.Message)})
	}
	head = append(head, nil)
	for _, row := range head {
		if err := w.Write(row); err != nil {
			return err
		}
	}

	if err := w.Write([]string{"Flags", "Control #", "Member", "Status", "Document", "Total", "Last year", "Sends to", "Line items", "Flag detail", "Decision"}); err != nil {
		return err
	}

	plans := make([]planner.MemberPlan, len(s.Plan.MemberPlans))
	copy(plans, s.Plan.MemberPlans)
	sort.SliceStable(plans, func(i, j int) bool {
		a, b := len(byMember[plans[i].Member.ControlNumber]), len(byMember[plans[j].Member.ControlNumber])
		if (a > 0) != (b > 0) {
			return a > 0
		}
		return plans[i].Member.Name < plans[j].Member.Name
	})

	for _, mp := range plans {
		flags := byMember[mp.Member.ControlNumber]
		if len(flags) == 0 && !creates(mp) && mp.Action != planner.ActionSkip {
			continue
		}
		row := []string{fmt.Sprint(len(flags)), mp.Member.ControlNumber, cell(mp.Member.Name), mp.Member.Status, documentName(mp), "", "", "", "", cell(strings.Join(flags, " | ")), ""}
		if mp.CalcResult != nil {
			row[5] = fmt.Sprintf("%.2f", mp.CalcResult.Total)
			parts := make([]string, len(mp.CalcResult.Lines))
			for i, l := range mp.CalcResult.Lines {
				parts[i] = fmt.Sprintf("%s %+.2f", l.Description, l.Amount)
			}
			row[8] = cell(strings.Join(parts, " | "))
		}
		if mp.QBOCustomer != nil {
			row[7] = cell(mp.QBOCustomer.Email)
			if doc, ok := last[mp.QBOCustomer.ID]; ok {
				row[6] = fmt.Sprintf("%.2f", doc.total)
			}
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func documentName(mp planner.MemberPlan) string {
	switch mp.Action {
	case planner.ActionCreate:
		return "Invoice"
	case planner.ActionCreateReceipt:
		return "$0 receipt"
	case planner.ActionSkip:
		return "Skipped"
	case planner.ActionError:
		return "Error"
	}
	return "None"
}

// cell stops a spreadsheet from treating member-supplied text as a formula.
func cell(s string) string {
	if s != "" && strings.ContainsAny(s[:1], "=+-@\t\r") {
		return "'" + s
	}
	return s
}

// Summary is the headline count of a plan.
type Summary struct {
	Invoices, Receipts, Skipped, Errors int
	Billed                              float64
}

// Summarize counts what a create plan would do.
func Summarize(plan *planner.Plan) Summary {
	var s Summary
	for _, mp := range plan.MemberPlans {
		switch mp.Action {
		case planner.ActionCreate:
			s.Invoices++
			s.Billed += mp.CalcResult.Total
		case planner.ActionCreateReceipt:
			s.Receipts++
		case planner.ActionSkip:
			s.Skipped++
		case planner.ActionError:
			s.Errors++
		}
	}
	return s
}

func stamp() string { return time.Now().Format(time.RFC3339) }
