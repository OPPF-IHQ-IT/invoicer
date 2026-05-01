package reconcile

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"github.com/willmadison/invoicer/internal/airtable"
	"github.com/willmadison/invoicer/internal/config"
)

// Options controls the reconciliation run.
type Options struct {
	QBOCustomersFile string
	DryRun           bool
	UpdateAirtable   bool
	Overwrite        bool
	AmbiguousOut     string
	MatchedOut       string
	UnmatchedOut     string
}

type qboCustomerRow struct {
	ID          string
	DisplayName string
	Email       string
}

type matchResult struct {
	Member      airtable.Member
	QBOCustomer *qboCustomerRow
	MatchedBy   string // "existing_id", "email", "name"
	Ambiguous   bool
	Unmatched   bool
}

// Customers reconciles the QBO CSV export against Airtable members.
func Customers(ctx context.Context, cfg *config.Config, opts Options) error {
	atClient := airtable.NewClient(cfg.Airtable.APIKey, cfg.Airtable.BaseID)

	members, err := atClient.ListAllMembers(ctx, &cfg.Airtable)
	if err != nil {
		return fmt.Errorf("loading Airtable members: %w", err)
	}

	qboCustomers, err := loadQBOCustomersCSV(opts.QBOCustomersFile)
	if err != nil {
		return fmt.Errorf("loading QBO customers CSV: %w", err)
	}

	byEmail := make(map[string][]*qboCustomerRow)
	byName := make(map[string][]*qboCustomerRow)
	for i := range qboCustomers {
		c := &qboCustomers[i]
		if c.Email != "" {
			key := strings.ToLower(strings.TrimSpace(c.Email))
			byEmail[key] = append(byEmail[key], c)
		}
		key := normalizeName(c.DisplayName)
		byName[key] = append(byName[key], c)
	}

	var results []matchResult
	stats := struct {
		total, existingID, byEmail, byName, ambiguous, unmatched, skipped int
	}{total: len(members)}

	for _, m := range members {
		// Skip members who already have a QBO Customer ID unless overwrite is set.
		if m.QBOCustomerID != "" && !opts.Overwrite {
			stats.skipped++
			continue
		}

		result := matchResult{Member: m}

		if m.QBOCustomerID != "" {
			stats.existingID++
			continue
		}

		emailKey := strings.ToLower(strings.TrimSpace(m.Email))
		if emailKey != "" {
			matches := byEmail[emailKey]
			switch len(matches) {
			case 1:
				result.QBOCustomer = matches[0]
				result.MatchedBy = "email"
				stats.byEmail++
			case 0:
				// fall through to name matching
			default:
				result.Ambiguous = true
				stats.ambiguous++
				results = append(results, result)
				continue
			}
		}

		if result.QBOCustomer == nil {
			nameKey := normalizeName(m.ControlNumber) // prefer control # key
			_ = nameKey
			// Try display name match as fallback.
			nameKey = normalizeName(m.Email) // placeholder — real name from Members table
			matches := byName[nameKey]
			switch len(matches) {
			case 1:
				result.QBOCustomer = matches[0]
				result.MatchedBy = "name"
				stats.byName++
			case 0:
				result.Unmatched = true
				stats.unmatched++
			default:
				result.Ambiguous = true
				stats.ambiguous++
			}
		}

		results = append(results, result)
	}

	// Write output CSV files.
	if err := writeResults(results, opts); err != nil {
		return err
	}

	// Update Airtable if requested and not dry-run.
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

	fmt.Printf("Reconciliation summary:\n")
	fmt.Printf("  Total Airtable members scanned:     %d\n", stats.total)
	fmt.Printf("  Total QBO customers loaded:          %d\n", len(qboCustomers))
	fmt.Printf("  Already had QBO Customer ID (skipped): %d\n", stats.skipped)
	fmt.Printf("  Matched by existing QBO Customer ID: %d\n", stats.existingID)
	fmt.Printf("  Matched by email:                    %d\n", stats.byEmail)
	fmt.Printf("  Matched by name:                     %d\n", stats.byName)
	fmt.Printf("  Ambiguous:                           %d\n", stats.ambiguous)
	fmt.Printf("  Unmatched:                           %d\n", stats.unmatched)
	if opts.DryRun {
		fmt.Println("\nDry run — no Airtable changes made.")
	}
	return nil
}

func loadQBOCustomersCSV(path string) ([]qboCustomerRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file has no data rows")
	}

	// Build header index.
	header := make(map[string]int)
	for i, h := range records[0] {
		header[strings.TrimSpace(h)] = i
	}

	idCol := findCol(header, "Id", "CustomerID", "Customer ID")
	nameCol := findCol(header, "Display Name", "DisplayName", "Name")
	emailCol := findCol(header, "Email", "Primary Email")

	var customers []qboCustomerRow
	for _, row := range records[1:] {
		c := qboCustomerRow{}
		if idCol >= 0 && idCol < len(row) {
			c.ID = strings.TrimSpace(row[idCol])
		}
		if nameCol >= 0 && nameCol < len(row) {
			c.DisplayName = strings.TrimSpace(row[nameCol])
		}
		if emailCol >= 0 && emailCol < len(row) {
			c.Email = strings.TrimSpace(row[emailCol])
		}
		if c.ID != "" {
			customers = append(customers, c)
		}
	}
	return customers, nil
}

func findCol(header map[string]int, candidates ...string) int {
	for _, c := range candidates {
		if i, ok := header[c]; ok {
			return i
		}
	}
	return -1
}

func normalizeName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func writeResults(results []matchResult, opts Options) error {
	if opts.MatchedOut != "" {
		var rows [][]string
		rows = append(rows, []string{"control_number", "email", "qbo_customer_id", "matched_by"})
		for _, r := range results {
			if !r.Ambiguous && !r.Unmatched && r.QBOCustomer != nil {
				rows = append(rows, []string{r.Member.ControlNumber, r.Member.Email, r.QBOCustomer.ID, r.MatchedBy})
			}
		}
		if err := writeCSV(opts.MatchedOut, rows); err != nil {
			return err
		}
	}

	if opts.AmbiguousOut != "" {
		var rows [][]string
		rows = append(rows, []string{"control_number", "email"})
		for _, r := range results {
			if r.Ambiguous {
				rows = append(rows, []string{r.Member.ControlNumber, r.Member.Email})
			}
		}
		if err := writeCSV(opts.AmbiguousOut, rows); err != nil {
			return err
		}
	}

	if opts.UnmatchedOut != "" {
		var rows [][]string
		rows = append(rows, []string{"control_number", "email"})
		for _, r := range results {
			if r.Unmatched {
				rows = append(rows, []string{r.Member.ControlNumber, r.Member.Email})
			}
		}
		if err := writeCSV(opts.UnmatchedOut, rows); err != nil {
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
