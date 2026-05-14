package campaign

import (
	"testing"
)

func TestLoadRowsFixture(t *testing.T) {
	rows, err := LoadRows("../../testdata/campaign-example.csv")
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if got, want := len(rows), 10; got != want {
		t.Fatalf("row count: got %d, want %d", got, want)
	}

	if rows[0].ControlNumber != "CT-0001" {
		t.Errorf("first row control number: got %q, want CT-0001", rows[0].ControlNumber)
	}
	if !rows[0].HasConsent() {
		t.Errorf("first row should have consent (%q)", rows[0].Consent)
	}
	amt, src, ok := rows[0].ResolveAmount()
	if !ok || amt != 150 || src != AmountFromStandard {
		t.Errorf("first row amount: got (%v, %v, %v), want (150, standard, true)", amt, src, ok)
	}

	// Other-amount row
	other := rows[3]
	if other.ControlNumber != "CT-0004" {
		t.Fatalf("expected CT-0004 at index 3, got %q", other.ControlNumber)
	}
	amt, src, ok = other.ResolveAmount()
	if !ok || amt != 75 || src != AmountFromOther {
		t.Errorf("other row amount: got (%v, %v, %v), want (75, form, true)", amt, src, ok)
	}

	// No-consent row
	noConsent := rows[4]
	if noConsent.HasConsent() {
		t.Errorf("no-consent row should not have consent (%q)", noConsent.Consent)
	}

	// Bad-amount row
	bad := rows[5]
	if _, _, ok := bad.ResolveAmount(); ok {
		t.Errorf("bad amount row should not resolve, got ok=true")
	}
}

func TestResolveAmountVariants(t *testing.T) {
	cases := []struct {
		name     string
		choice   string
		other    string
		want     float64
		wantSrc  AmountSource
		wantOK   bool
	}{
		{"standard 150", "$150 — Standard", "", 150, AmountFromStandard, true},
		{"standard 300", "$300 — Sustaining Supporter", "", 300, AmountFromStandard, true},
		{"standard 500", "$500 — Facility Champion", "", 500, AmountFromStandard, true},
		{"other 75", "Other:", "75", 75, AmountFromOther, true},
		{"other with $ and commas", "Other:", "$1,250.50", 1250.50, AmountFromOther, true},
		{"other zero", "Other:", "0", 0, AmountFromOther, false},
		{"other negative", "Other:", "-10", 0, AmountFromOther, false},
		{"other unparseable", "Other:", "abc", 0, AmountFromOther, false},
		{"choice without dollar", "no idea", "", 0, AmountFromStandard, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Row{InvoiceAmountChoice: c.choice, RequestedAmountOther: c.other}
			amt, src, ok := r.ResolveAmount()
			if ok != c.wantOK {
				t.Errorf("ok: got %v, want %v", ok, c.wantOK)
			}
			if ok && amt != c.want {
				t.Errorf("amount: got %v, want %v", amt, c.want)
			}
			if ok && src != c.wantSrc {
				t.Errorf("source: got %v, want %v", src, c.wantSrc)
			}
		})
	}
}

func TestHasConsent(t *testing.T) {
	cases := map[string]bool{
		"":         false,
		"Yes":      true,
		"yes":      true,
		"I agree":  true,
		"Agreed":   true,
		"no":       false,
		"N":        false,
		"false":    false,
		"0":        false,
	}
	for v, want := range cases {
		r := Row{Consent: v}
		if got := r.HasConsent(); got != want {
			t.Errorf("HasConsent(%q): got %v, want %v", v, got, want)
		}
	}
}

func TestDedupeKeepsLatest(t *testing.T) {
	rows, err := LoadRows("../../testdata/campaign-example.csv")
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	deduped := dedupeByControlNumber(rows)
	if got, want := len(deduped), 9; got != want {
		t.Fatalf("deduped count: got %d, want %d", got, want)
	}
	// CT-0007 appears twice in fixture; later submission is $300 (Sustaining).
	var greg *Row
	for i := range deduped {
		if deduped[i].ControlNumber == "CT-0007" {
			greg = &deduped[i]
		}
	}
	if greg == nil {
		t.Fatal("CT-0007 missing from deduped set")
	}
	amt, _, ok := greg.ResolveAmount()
	if !ok || amt != 300 {
		t.Errorf("CT-0007 winner amount: got (%v, ok=%v), want 300", amt, ok)
	}

	superseded := SupersededRows(rows)
	if got, want := len(superseded), 1; got != want {
		t.Fatalf("superseded count: got %d, want %d", got, want)
	}
	if superseded[0].ControlNumber != "CT-0007" {
		t.Errorf("superseded control number: got %q, want CT-0007", superseded[0].ControlNumber)
	}
}
