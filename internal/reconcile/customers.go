package reconcile

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// Options controls the reconciliation run.
type Options struct {
	DryRun          bool
	UpdateAirtable  bool
	Overwrite       bool
	CreateMissing   bool // create QBO customers for unmatched members who have an email
	AmbiguousOut    string
	MatchedOut      string
	UnmatchedOut    string
	SkippedOut      string
}

type matchResult struct {
	Member      airtable.Member
	QBOCustomer *qbo.Customer
	MatchedBy   string // "control_number", "email"
	Ambiguous   bool
	Unmatched   bool
	NoEmail     bool
	NoLongerMember bool
}

// Customers reconciles QBO customers (fetched via API) against Airtable members.
func Customers(ctx context.Context, cfg *config.Config, opts Options) error {
	atClient := airtable.NewClient(cfg.Airtable.APIKey, cfg.Airtable.BaseID)

	members, err := atClient.ListAllMembers(ctx, &cfg.Airtable)
	if err != nil {
		return fmt.Errorf("loading Airtable members: %w", err)
	}

	qboClient, err := qbo.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("connecting to QBO: %w", err)
	}

	customers, err := qboClient.ListCustomers(ctx)
	if err != nil {
		return fmt.Errorf("loading QBO customers: %w", err)
	}

	// Index customers by normalised email and by normalised Notes (control #).
	byEmail := make(map[string][]*qbo.Customer)
	byControlNumber := make(map[string][]*qbo.Customer)
	for i := range customers {
		c := &customers[i]
		if c.Email != "" {
			byEmail[normalise(c.Email)] = append(byEmail[normalise(c.Email)], c)
		}
		if c.Notes != "" {
			byControlNumber[normalise(c.Notes)] = append(byControlNumber[normalise(c.Notes)], c)
		}
	}

	var results []matchResult
	stats := struct {
		total, skipped, noLongerMember, byControlNumber, byEmail, ambiguous, unmatched, noEmail, created int
	}{total: len(members)}

	for _, m := range members {
		if m.Status == cfg.Airtable.StatusValues.NoLongerMember {
			stats.noLongerMember++
			results = append(results, matchResult{Member: m, NoLongerMember: true})
			continue
		}

		if m.QBOCustomerID != "" && !opts.Overwrite {
			stats.skipped++
			continue
		}

		if m.Email == "" {
			stats.noEmail++
			results = append(results, matchResult{Member: m, NoEmail: true})
			continue
		}

		result := matchResult{Member: m}

		// Match by control number stored in QBO Notes field.
		if m.ControlNumber != "" {
			matches := byControlNumber[normalise(m.ControlNumber)]
			switch len(matches) {
			case 1:
				result.QBOCustomer = matches[0]
				result.MatchedBy = "control_number"
				stats.byControlNumber++
				results = append(results, result)
				continue
			case 0:
				// fall through to email matching
			default:
				result.Ambiguous = true
				stats.ambiguous++
				results = append(results, result)
				continue
			}
		}

		// Match by email.
		matches := byEmail[normalise(m.Email)]
		switch len(matches) {
		case 1:
			result.QBOCustomer = matches[0]
			result.MatchedBy = "email"
			stats.byEmail++
		case 0:
			result.Unmatched = true
			stats.unmatched++
		default:
			result.Ambiguous = true
			stats.ambiguous++
		}

		results = append(results, result)
	}

	if err := writeResults(results, opts); err != nil {
		return err
	}

	if opts.UpdateAirtable && !opts.DryRun {
		for _, r := range results {
			if r.Ambiguous || r.Unmatched || r.QBOCustomer == nil {
				continue
			}
			if r.Member.QBOCustomerID != "" && !opts.Overwrite {
				continue
			}
			if err := atClient.UpdateMemberQBOCustomerID(ctx, &cfg.Airtable, r.Member.RecordID, r.QBOCustomer.ID); err != nil {
				fmt.Fprintf(os.Stderr, "warning: updating QBO Customer ID for %s: %v\n", r.Member.ControlNumber, err)
			}
		}
	}

	if opts.CreateMissing && !opts.DryRun {
		for i, r := range results {
			if !r.Unmatched || r.Member.Email == "" {
				continue
			}
			displayName := r.Member.Name
			if displayName == "" {
				displayName = r.Member.Email
			}
			cust, err := qboClient.CreateCustomer(ctx, displayName, r.Member.Email, r.Member.ControlNumber)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: creating QBO customer for %s: %v\n", r.Member.ControlNumber, err)
				continue
			}
			if opts.UpdateAirtable {
				if err := atClient.UpdateMemberQBOCustomerID(ctx, &cfg.Airtable, r.Member.RecordID, cust.ID); err != nil {
					fmt.Fprintf(os.Stderr, "warning: updating QBO Customer ID for %s: %v\n", r.Member.ControlNumber, err)
				}
			}
			results[i].QBOCustomer = cust
			results[i].Unmatched = false
			results[i].MatchedBy = "created"
			stats.unmatched--
			stats.created++
			fmt.Printf("  Created QBO customer for %s (%s) → ID %s\n", r.Member.ControlNumber, r.Member.Email, cust.ID)
		}
	}

	fmt.Printf("Reconciliation summary:\n")
	fmt.Printf("  Total Airtable members scanned:        %d\n", stats.total)
	fmt.Printf("  Total QBO customers loaded:            %d\n", len(customers))
	fmt.Printf("  No longer a member of this chapter:    %d\n", stats.noLongerMember)
	fmt.Printf("  Already had QBO Customer ID (skipped): %d\n", stats.skipped)
	fmt.Printf("  Skipped — no email address:            %d\n", stats.noEmail)
	fmt.Printf("  Matched by control number:             %d\n", stats.byControlNumber)
	fmt.Printf("  Matched by email:                      %d\n", stats.byEmail)
	fmt.Printf("  Ambiguous:                             %d\n", stats.ambiguous)
	fmt.Printf("  Unmatched:                             %d\n", stats.unmatched)
	fmt.Printf("  Created in QBO:                        %d\n", stats.created)
	if opts.DryRun {
		fmt.Println("\nDry run — no Airtable changes made.")
	}
	return nil
}

func normalise(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func writeResults(results []matchResult, opts Options) error {
	if opts.MatchedOut != "" {
		var rows [][]string
		rows = append(rows, []string{"control_number", "status", "email", "qbo_customer_id", "qbo_display_name", "matched_by"})
		for _, r := range results {
			if !r.Ambiguous && !r.Unmatched && r.QBOCustomer != nil {
				rows = append(rows, []string{r.Member.ControlNumber, r.Member.Status, r.Member.Email, r.QBOCustomer.ID, r.QBOCustomer.DisplayName, r.MatchedBy})
			}
		}
		if err := writeCSV(opts.MatchedOut, rows); err != nil {
			return err
		}
	}

	if opts.AmbiguousOut != "" {
		var rows [][]string
		rows = append(rows, []string{"control_number", "status", "email"})
		for _, r := range results {
			if r.Ambiguous {
				rows = append(rows, []string{r.Member.ControlNumber, r.Member.Status, r.Member.Email})
			}
		}
		if err := writeCSV(opts.AmbiguousOut, rows); err != nil {
			return err
		}
	}

	if opts.UnmatchedOut != "" {
		var rows [][]string
		rows = append(rows, []string{"control_number", "status", "email"})
		for _, r := range results {
			if r.Unmatched {
				rows = append(rows, []string{r.Member.ControlNumber, r.Member.Status, r.Member.Email})
			}
		}
		if err := writeCSV(opts.UnmatchedOut, rows); err != nil {
			return err
		}
	}

	if opts.SkippedOut != "" {
		var rows [][]string
		rows = append(rows, []string{"control_number", "status", "reason"})
		for _, r := range results {
			switch {
			case r.NoLongerMember:
				rows = append(rows, []string{r.Member.ControlNumber, r.Member.Status, "no longer a member of this chapter"})
			case r.NoEmail:
				rows = append(rows, []string{r.Member.ControlNumber, r.Member.Status, "no email address"})
			}
		}
		if err := writeCSV(opts.SkippedOut, rows); err != nil {
			return err
		}
	}

	return nil
}

func writeCSV(path string, rows [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	return w.WriteAll(rows)
}
