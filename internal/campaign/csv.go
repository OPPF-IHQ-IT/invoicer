package campaign

import (
	"encoding/csv"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Row is one parsed entry from the Google Form CSV export.
type Row struct {
	LineNumber                 int
	Timestamp                  time.Time
	FullName                   string
	Email                      string
	ControlNumber              string
	InvoiceAmountChoice        string // raw value from the "Invoice Amount" column
	RequestedAmountOther       string // raw value from the "Please enter your requested invoice amount" column
	Consent                    string // raw value from the "Consent/Authorization" column
}

// expected column headers, in any order, from the Google Form export
const (
	colTimestamp     = "Timestamp"
	colFullName      = "Full Name"
	colEmail         = "Email Address"
	colControlNumber = "Control Number"
	colInvoiceAmount = "Invoice Amount"
	colOtherAmount   = "Please enter your requested invoice amount"
	colConsent       = "Consent/Authorization"
)

var requiredColumns = []string{
	colTimestamp,
	colFullName,
	colEmail,
	colControlNumber,
	colInvoiceAmount,
	colOtherAmount,
	colConsent,
}

// LoadRows reads and parses a Google Form CSV export from path.
// Header validation is strict — missing columns return an error so a drifted
// export fails loudly instead of silently dropping data.
func LoadRows(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening CSV %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolerate trailing empties

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading CSV header: %w", err)
	}

	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[strings.TrimSpace(h)] = i
	}
	for _, col := range requiredColumns {
		if _, ok := idx[col]; !ok {
			return nil, fmt.Errorf("CSV missing required column %q", col)
		}
	}

	var rows []Row
	lineNum := 1 // header is line 1
	for {
		rec, err := r.Read()
		if err != nil {
			break
		}
		lineNum++

		get := func(col string) string {
			i := idx[col]
			if i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}

		ts, _ := parseFormTimestamp(get(colTimestamp))
		rows = append(rows, Row{
			LineNumber:           lineNum,
			Timestamp:            ts,
			FullName:             get(colFullName),
			Email:                get(colEmail),
			ControlNumber:        get(colControlNumber),
			InvoiceAmountChoice:  get(colInvoiceAmount),
			RequestedAmountOther: get(colOtherAmount),
			Consent:              get(colConsent),
		})
	}

	return rows, nil
}

// parseFormTimestamp accepts the formats Google Forms emits for the Timestamp
// column. Returns zero time on failure — callers fall back to line order for
// tie-breaking duplicates.
func parseFormTimestamp(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty")
	}
	layouts := []string{
		"1/2/2006 15:04:05",
		"2006-01-02 15:04:05",
		"1/2/2006 15:04",
		"2006-01-02T15:04:05Z",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized timestamp %q", s)
}

// AmountSource describes which CSV column supplied the resolved amount.
type AmountSource string

const (
	AmountFromStandard AmountSource = "standard" // parsed from the "Invoice Amount" choice
	AmountFromOther    AmountSource = "form"     // parsed from the "Please enter..." column
)

var leadingDollarRe = regexp.MustCompile(`\$?\s*([0-9]+(?:\.[0-9]+)?)`)

// ResolveAmount returns the dollar amount the brother declared, plus which
// column it came from. Empty other-column → parse the leading "$N" from the
// choice column. Returns ok=false for unparseable / non-positive values.
func (r Row) ResolveAmount() (amount float64, source AmountSource, ok bool) {
	if v := strings.TrimSpace(r.RequestedAmountOther); v != "" {
		clean := strings.ReplaceAll(strings.ReplaceAll(v, "$", ""), ",", "")
		n, err := strconv.ParseFloat(strings.TrimSpace(clean), 64)
		if err != nil || n <= 0 {
			return 0, AmountFromOther, false
		}
		return n, AmountFromOther, true
	}
	m := leadingDollarRe.FindStringSubmatch(r.InvoiceAmountChoice)
	if len(m) < 2 {
		return 0, AmountFromStandard, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil || n <= 0 {
		return 0, AmountFromStandard, false
	}
	return n, AmountFromStandard, true
}

// HasConsent reports whether the consent column should be treated as opted-in.
// The Google Form makes the field required, so an empty value implies a
// malformed export rather than a real opt-out.
func (r Row) HasConsent() bool {
	v := strings.ToLower(strings.TrimSpace(r.Consent))
	if v == "" {
		return false
	}
	switch v {
	case "no", "n", "false", "0":
		return false
	}
	return true
}
