package fiscal

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Year represents a parsed, canonical fiscal year.
type Year struct {
	Label      string    // FY2026
	Number     int       // 2026
	PeriodStart time.Time // 2025-11-01
	PeriodEnd   time.Time // 2026-10-31
	// AirtableLabel is the Airtable-formatted range label, e.g. "2025-2026".
	AirtableLabel string
	// OnTimePaymentEnd is the last day dues are considered on-time.
	OnTimePaymentEnd time.Time
	// LateFeeStartsOn is the first day late fees apply.
	LateFeeStartsOn time.Time
}

// Parse accepts "FY2026" or "2026" and returns a canonical Year.
func Parse(s string) (Year, error) {
	s = strings.TrimSpace(s)
	numStr := strings.TrimPrefix(s, "FY")
	n, err := strconv.Atoi(numStr)
	if err != nil || n < 2000 || n > 2100 {
		return Year{}, fmt.Errorf("expected format FY2026 or 2026, got %q", s)
	}
	return fromNumber(n), nil
}

func fromNumber(n int) Year {
	periodStart := time.Date(n-1, time.November, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := time.Date(n, time.October, 31, 0, 0, 0, 0, time.UTC)
	return Year{
		Label:            fmt.Sprintf("FY%d", n),
		Number:           n,
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		AirtableLabel:    fmt.Sprintf("%d-%d", n-1, n),
		OnTimePaymentEnd: time.Date(n-1, time.December, 31, 0, 0, 0, 0, time.UTC),
		LateFeeStartsOn:  time.Date(n, time.January, 1, 0, 0, 0, 0, time.UTC),
	}
}

// Contains reports whether the given date falls within this fiscal year's period.
func (y Year) Contains(t time.Time) bool {
	d := t.Truncate(24 * time.Hour)
	return !d.Before(y.PeriodStart) && !d.After(y.PeriodEnd)
}

// IsLate reports whether the as-of date is on or after the late fee start date.
func (y Year) IsLate(asOf time.Time) bool {
	return !asOf.Before(y.LateFeeStartsOn)
}
