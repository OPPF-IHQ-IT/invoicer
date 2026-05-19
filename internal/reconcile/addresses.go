package reconcile

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
)

// AddressImportOptions controls an addresses-import run.
type AddressImportOptions struct {
	CSVPath      string
	DryRun       bool
	ProcessedOut string // optional path: writes a mirror of the input with an "Updated in Airtable?" column populated
}

// Google Form column headers we care about. Matched post-TrimSpace, case-sensitive.
const (
	colAddrControlNumber = "Control #"
	colAddrEmail         = "Primary Email Address"
	colAddrLine1         = "Mailing Address Line 1"
	colAddrLine2         = "Mailing Address Line 2"
	colAddrCity          = "City"
	colAddrState         = "State"
	colAddrZip           = "Zip"
)

var addrRequiredColumns = []string{
	colAddrControlNumber,
	colAddrEmail,
	colAddrLine1,
	colAddrLine2,
	colAddrCity,
	colAddrState,
	colAddrZip,
}

// addressRow holds the fields we extract from one CSV line.
type addressRow struct {
	rawRecord     []string // full record, for processed-out mirroring
	controlNumber string
	email         string
	line1         string
	line2         string
	city          string
	state         string // normalised to USPS two-letter when possible
	stateRaw      string // original CSV value (for the report when normalisation fails)
	stateOK       bool   // true if NormalizeState succeeded
	zip           string
}

// ImportAddresses reads a Google Form CSV export, matches each row to an
// Airtable member by Control #, and PATCHes mailing-address fields onto the
// member record. Email is only written when the Airtable email is blank;
// existing emails are left alone whether they match or not.
func ImportAddresses(ctx context.Context, cfg *config.Config, opts AddressImportOptions) error {
	if opts.CSVPath == "" {
		return fmt.Errorf("csv path is required")
	}

	rows, header, err := loadAddressCSV(opts.CSVPath)
	if err != nil {
		return err
	}

	atClient := airtable.NewClient(cfg.Airtable.APIKey, cfg.Airtable.BaseID)
	members, err := atClient.ListAllMembers(ctx, &cfg.Airtable)
	if err != nil {
		return fmt.Errorf("loading Airtable members: %w", err)
	}

	byControl := make(map[string]*airtable.Member, len(members))
	for i := range members {
		k := normalise(members[i].ControlNumber)
		if k == "" {
			continue
		}
		byControl[k] = &members[i]
	}

	type rowOutcome struct {
		row     addressRow
		status  string // "updated", "no-change", "unmatched", "skipped"
		reason  string
		patched map[string]any
	}

	outcomes := make([]rowOutcome, 0, len(rows))
	stats := struct {
		total, updated, noChange, unmatched, skipped, stateFlagged int
	}{total: len(rows)}

	f := cfg.Airtable.Fields.Members

	for _, row := range rows {
		out := rowOutcome{row: row}

		if row.controlNumber == "" {
			out.status = "skipped"
			out.reason = "missing control number"
			stats.skipped++
			outcomes = append(outcomes, out)
			continue
		}

		member, ok := byControl[normalise(row.controlNumber)]
		if !ok {
			out.status = "unmatched"
			out.reason = "no Airtable member with this Control #"
			stats.unmatched++
			outcomes = append(outcomes, out)
			continue
		}

		patch := make(map[string]any)

		// Email: only write if Airtable's value is blank.
		if strings.TrimSpace(member.Email) == "" && row.email != "" {
			patch[f.Email] = row.email
		}

		// Address fields: write when the CSV provides a value AND it differs from current.
		// Empty CSV cells are skipped (we never blank a populated field).
		if row.line1 != "" && row.line1 != member.AddressLine1 {
			patch[f.AddressLine1] = row.line1
		}
		if row.line2 != "" && row.line2 != member.AddressLine2 {
			patch[f.AddressLine2] = row.line2
		}
		if row.city != "" && row.city != member.City {
			patch[f.City] = row.city
		}
		if row.state != "" && row.state != member.State {
			patch[f.State] = row.state
		}
		if row.zip != "" && row.zip != member.Zip {
			patch[f.Zip] = row.zip
		}

		if !row.stateOK && row.stateRaw != "" {
			stats.stateFlagged++
		}

		if len(patch) == 0 {
			out.status = "no-change"
			stats.noChange++
			outcomes = append(outcomes, out)
			continue
		}

		out.patched = patch

		if opts.DryRun {
			out.status = "updated"
			out.reason = "dry-run (not written)"
			stats.updated++
			outcomes = append(outcomes, out)
			continue
		}

		if err := atClient.UpdateMemberFields(ctx, &cfg.Airtable, member.RecordID, patch); err != nil {
			out.status = "skipped"
			out.reason = fmt.Sprintf("PATCH failed: %v", err)
			stats.skipped++
			fmt.Fprintf(os.Stderr, "warning: updating member %s: %v\n", row.controlNumber, err)
			outcomes = append(outcomes, out)
			continue
		}

		out.status = "updated"
		stats.updated++
		outcomes = append(outcomes, out)
	}

	if opts.ProcessedOut != "" {
		processedHeader := append([]string{}, header...)
		// Replace or append the "Updated in Airtable?" column with our outcome.
		updatedColIdx := -1
		for i, h := range processedHeader {
			if strings.TrimSpace(h) == "Updated in Airtable?" {
				updatedColIdx = i
				break
			}
		}
		if updatedColIdx == -1 {
			processedHeader = append(processedHeader, "Updated in Airtable?")
		}
		processedRows := [][]string{processedHeader}
		for _, o := range outcomes {
			rec := append([]string{}, o.row.rawRecord...)
			// Pad to header length so column indexing is safe.
			for len(rec) < len(processedHeader) {
				rec = append(rec, "")
			}
			label := outcomeLabel(o.status, o.reason, o.row)
			idx := updatedColIdx
			if idx == -1 {
				idx = len(processedHeader) - 1
			}
			if idx < len(rec) {
				rec[idx] = label
			}
			processedRows = append(processedRows, rec)
		}
		if err := writeCSV(opts.ProcessedOut, processedRows); err != nil {
			return fmt.Errorf("writing processed CSV: %w", err)
		}
	}

	fmt.Printf("Address import summary:\n")
	fmt.Printf("  Total CSV rows:                %d\n", stats.total)
	fmt.Printf("  Updated:                       %d\n", stats.updated)
	fmt.Printf("  No change (already current):   %d\n", stats.noChange)
	fmt.Printf("  Unmatched (no member found):   %d\n", stats.unmatched)
	fmt.Printf("  Skipped (error / missing key): %d\n", stats.skipped)
	fmt.Printf("  Unrecognised state values:     %d (written as-is)\n", stats.stateFlagged)
	if opts.DryRun {
		fmt.Println("\nDry run — no Airtable changes made.")
	}
	return nil
}

func outcomeLabel(status, reason string, r addressRow) string {
	label := status
	notes := []string{}
	if reason != "" {
		notes = append(notes, reason)
	}
	if !r.stateOK && r.stateRaw != "" {
		notes = append(notes, fmt.Sprintf("state %q not normalised", r.stateRaw))
	}
	if len(notes) > 0 {
		label += " — " + strings.Join(notes, "; ")
	}
	return label
}

// loadAddressCSV reads the Google Form export and returns parsed rows plus the
// original header (for processed-out mirroring). All cells are TrimSpace'd.
func loadAddressCSV(path string) ([]addressRow, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("opening CSV %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("reading CSV header: %w", err)
	}

	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[strings.TrimSpace(h)] = i
	}
	for _, col := range addrRequiredColumns {
		if _, ok := idx[col]; !ok {
			return nil, nil, fmt.Errorf("CSV missing required column %q", col)
		}
	}

	get := func(rec []string, col string) string {
		i, ok := idx[col]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}

	var rows []addressRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("reading CSV row: %w", err)
		}

		stateRaw := get(rec, colAddrState)
		normalized, ok := NormalizeState(stateRaw)

		rows = append(rows, addressRow{
			rawRecord:     append([]string{}, rec...),
			controlNumber: get(rec, colAddrControlNumber),
			email:         get(rec, colAddrEmail),
			line1:         get(rec, colAddrLine1),
			line2:         get(rec, colAddrLine2),
			city:          get(rec, colAddrCity),
			state:         normalized,
			stateRaw:      stateRaw,
			stateOK:       ok,
			zip:           get(rec, colAddrZip),
		})
	}

	return rows, header, nil
}
