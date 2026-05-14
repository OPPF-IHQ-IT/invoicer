package campaign

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
)

// MatchedRow is a CSV row that survived reconciliation and is ready to invoice.
type MatchedRow struct {
	Row             Row
	Member          airtable.Member
	Amount          float64
	AmountSource    AmountSource
	NeedsQBOCreate  bool // Airtable record exists but lacks a QBO Customer ID
	// CreationEmail / CreationName are the values to pass to qbo.CreateCustomer
	// when NeedsQBOCreate is true. Prefer Airtable values, fall back to form.
	CreationEmail string
	CreationName  string
}

// UnmatchedRow is a row we couldn't safely invoice (no Airtable record, or
// Airtable record without a QBO customer and no way to create one).
type UnmatchedRow struct {
	Row    Row
	Reason string // "no_airtable_record", "no_qbo_customer_id", "no_email_for_creation"
}

// SkippedRow is a row excluded before reconciliation (consent, bad amount, ex-member).
type SkippedRow struct {
	Row    Row
	Reason string // "no_consent", "bad_amount", "no_longer_member", "duplicate_superseded"
}

// Result is the output of Reconcile — three buckets the caller renders + acts on.
type Result struct {
	Matched   []MatchedRow
	Unmatched []UnmatchedRow
	Skipped   []SkippedRow
}

// Options shapes reconciliation behavior.
type Options struct {
	// CreateMissing controls whether matched rows whose Airtable record has no
	// QBO Customer ID are eligible to have a QBO customer created downstream.
	// When false, they bucket as unmatched (no_qbo_customer_id).
	CreateMissing bool
}

// Reconcile groups CSV rows against Airtable members.
//
// The function does not touch QBO. It pulls Airtable members once, indexes by
// control number, applies the per-row gates (consent, amount, status), and
// produces the three result buckets. Duplicate control numbers in the input
// are deduped to the latest by timestamp; losers bucket as skipped.
func Reconcile(ctx context.Context, cfg *config.Config, rows []Row, opts Options) (*Result, error) {
	atClient := airtable.NewClient(cfg.Airtable.APIKey, cfg.Airtable.BaseID)
	members, err := atClient.ListAllMembers(ctx, &cfg.Airtable)
	if err != nil {
		return nil, fmt.Errorf("loading Airtable members: %w", err)
	}

	byControl := make(map[string]airtable.Member, len(members))
	for _, m := range members {
		key := normalizeControlNumber(m.ControlNumber)
		if key == "" {
			continue
		}
		byControl[key] = m
	}

	rows = dedupeByControlNumber(rows)

	res := &Result{}
	for _, row := range rows {
		if !row.HasConsent() {
			res.Skipped = append(res.Skipped, SkippedRow{Row: row, Reason: "no_consent"})
			continue
		}
		amount, source, ok := row.ResolveAmount()
		if !ok {
			res.Skipped = append(res.Skipped, SkippedRow{Row: row, Reason: "bad_amount"})
			continue
		}

		ctrl := normalizeControlNumber(row.ControlNumber)
		member, found := byControl[ctrl]
		if !found {
			res.Unmatched = append(res.Unmatched, UnmatchedRow{Row: row, Reason: "no_airtable_record"})
			continue
		}

		if member.Status == cfg.Airtable.StatusValues.NoLongerMember {
			res.Skipped = append(res.Skipped, SkippedRow{Row: row, Reason: "no_longer_member"})
			continue
		}

		match := MatchedRow{
			Row:          row,
			Member:       member,
			Amount:       amount,
			AmountSource: source,
		}

		if member.QBOCustomerID == "" {
			if !opts.CreateMissing {
				res.Unmatched = append(res.Unmatched, UnmatchedRow{Row: row, Reason: "no_qbo_customer_id"})
				continue
			}
			email := strings.TrimSpace(member.Email)
			if email == "" {
				email = strings.TrimSpace(row.Email)
			}
			if email == "" {
				res.Unmatched = append(res.Unmatched, UnmatchedRow{Row: row, Reason: "no_email_for_creation"})
				continue
			}
			name := strings.TrimSpace(member.Name)
			if name == "" {
				name = strings.TrimSpace(row.FullName)
			}
			if name == "" {
				name = email
			}
			match.NeedsQBOCreate = true
			match.CreationEmail = email
			match.CreationName = name
		}

		res.Matched = append(res.Matched, match)
	}

	return res, nil
}

// dedupeByControlNumber keeps the latest submission (by Timestamp, then file
// order) when the same control number appears multiple times. Losers are
// silently dropped from the input — the caller layers a "duplicate_superseded"
// skip entry on top if desired. We don't add it here because Reconcile's
// gating below would still need to run against the winner.
func dedupeByControlNumber(rows []Row) []Row {
	type indexed struct {
		row Row
		i   int
	}
	groups := make(map[string][]indexed)
	for i, r := range rows {
		key := normalizeControlNumber(r.ControlNumber)
		if key == "" {
			groups[fmt.Sprintf("__line_%d", i)] = []indexed{{r, i}}
			continue
		}
		groups[key] = append(groups[key], indexed{r, i})
	}

	out := make([]Row, 0, len(rows))
	for _, g := range groups {
		if len(g) == 1 {
			out = append(out, g[0].row)
			continue
		}
		sort.SliceStable(g, func(i, j int) bool {
			if g[i].row.Timestamp.Equal(g[j].row.Timestamp) {
				return g[i].i < g[j].i
			}
			return g[i].row.Timestamp.Before(g[j].row.Timestamp)
		})
		out = append(out, g[len(g)-1].row)
	}
	// Restore original-ish ordering by line number for stable reports.
	sort.SliceStable(out, func(i, j int) bool { return out[i].LineNumber < out[j].LineNumber })
	return out
}

// SupersededRows returns the rows that were dropped by dedupeByControlNumber
// (i.e. older submissions for a control number that had a later one). The
// caller can render these into the skipped CSV with reason
// "duplicate_superseded" so operators see the full input.
func SupersededRows(rows []Row) []Row {
	type indexed struct {
		row Row
		i   int
	}
	groups := make(map[string][]indexed)
	for i, r := range rows {
		key := normalizeControlNumber(r.ControlNumber)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], indexed{r, i})
	}

	var out []Row
	for _, g := range groups {
		if len(g) <= 1 {
			continue
		}
		sort.SliceStable(g, func(i, j int) bool {
			if g[i].row.Timestamp.Equal(g[j].row.Timestamp) {
				return g[i].i < g[j].i
			}
			return g[i].row.Timestamp.Before(g[j].row.Timestamp)
		})
		for _, idx := range g[:len(g)-1] {
			out = append(out, idx.row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LineNumber < out[j].LineNumber })
	return out
}

func normalizeControlNumber(s string) string {
	return strings.TrimSpace(s)
}
