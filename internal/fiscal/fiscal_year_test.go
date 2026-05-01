package fiscal

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	cases := []struct {
		input       string
		wantLabel   string
		wantNumber  int
		wantStart   string
		wantEnd     string
		wantAirtable string
		wantErr     bool
	}{
		{"FY2026", "FY2026", 2026, "2025-11-01", "2026-10-31", "2025-2026", false},
		{"2026", "FY2026", 2026, "2025-11-01", "2026-10-31", "2025-2026", false},
		{"FY2027", "FY2027", 2027, "2026-11-01", "2027-10-31", "2026-2027", false},
		{"", "", 0, "", "", "", true},
		{"FY19xx", "", 0, "", "", "", true},
		{"1999", "", 0, "", "", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			fy, err := Parse(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got none", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fy.Label != tc.wantLabel {
				t.Errorf("Label: got %q, want %q", fy.Label, tc.wantLabel)
			}
			if fy.Number != tc.wantNumber {
				t.Errorf("Number: got %d, want %d", fy.Number, tc.wantNumber)
			}
			if got := fy.PeriodStart.Format("2006-01-02"); got != tc.wantStart {
				t.Errorf("PeriodStart: got %q, want %q", got, tc.wantStart)
			}
			if got := fy.PeriodEnd.Format("2006-01-02"); got != tc.wantEnd {
				t.Errorf("PeriodEnd: got %q, want %q", got, tc.wantEnd)
			}
			if fy.AirtableLabel != tc.wantAirtable {
				t.Errorf("AirtableLabel: got %q, want %q", fy.AirtableLabel, tc.wantAirtable)
			}
		})
	}
}

func TestIsLate(t *testing.T) {
	fy, _ := Parse("FY2026")
	cases := []struct {
		asOf     string
		wantLate bool
	}{
		{"2025-11-01", false},
		{"2025-12-31", false},
		{"2026-01-01", true},
		{"2026-06-15", true},
	}
	for _, tc := range cases {
		d, _ := time.Parse("2006-01-02", tc.asOf)
		if got := fy.IsLate(d); got != tc.wantLate {
			t.Errorf("IsLate(%s): got %v, want %v", tc.asOf, got, tc.wantLate)
		}
	}
}
