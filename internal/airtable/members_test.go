package airtable

import (
	"testing"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
)

func TestIsHomeChapter(t *testing.T) {
	cases := []struct {
		got, home string
		want      bool
	}{
		{"Chi Tau", "Chi Tau", true},
		{"  chi   TAU ", "Chi Tau", true},
		{"Alpha", "Chi Tau", false},
		{"", "Chi Tau", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := isHomeChapter(c.got, c.home); got != c.want {
			t.Errorf("isHomeChapter(%q, %q) = %v, want %v", c.got, c.home, got, c.want)
		}
	}
}

func TestBoolField(t *testing.T) {
	r := record{Fields: map[string]any{
		"checkbox":     true,
		"formulaCheck": "✅",
		"formulaBlank": "",
		"formulaFalse": "false",
		"number":       float64(1),
		"zero":         float64(0),
	}}
	cases := map[string]bool{
		"checkbox": true, "formulaCheck": true, "formulaBlank": false,
		"formulaFalse": false, "number": true, "zero": false, "missing": false,
	}
	for key, want := range cases {
		if got := boolField(r, key); got != want {
			t.Errorf("boolField(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestParseMemberCredits(t *testing.T) {
	f := config.MemberCreditsFieldsConfig{
		MemberLink: "Members", Type: "Type", AppliesTo: "Applies To",
		Earned: "Earned", Spent: "Spent", MaxPerYear: "Max Per Year",
	}
	records := []record{
		{ID: "recNewer", CreatedTime: "2026-10-16T00:00:00.000Z", Fields: map[string]any{
			"Members": []any{"recKen"}, "Type": "Overpayment", "Applies To": "Total Due",
			"Earned": float64(377), "Spent": float64(98),
		}},
		{ID: "recOlder", CreatedTime: "2025-01-01T00:00:00.000Z", Fields: map[string]any{
			"Members": []any{"recKen"}, "Applies To": " local only ", "Earned": float64(50), "Max Per Year": float64(25),
		}},
		{ID: "recSpent", Fields: map[string]any{
			"Members": []any{"recKen"}, "Earned": float64(100), "Spent": float64(100),
		}},
		{ID: "recUnlinked", Fields: map[string]any{"Earned": float64(100)}},
		{ID: "recShared", Fields: map[string]any{
			"Members": []any{"recA", "recB"}, "Earned": float64(10),
		}},
	}
	got := parseMemberCredits(records, f)

	ken := got["recKen"]
	if len(ken) != 2 || ken[0].RecordID != "recOlder" || ken[1].RecordID != "recNewer" {
		t.Fatalf("recKen credits: got %+v, want recOlder then recNewer", ken)
	}
	if !ken[0].LocalOnly || ken[0].MaxPerYear != 25 || ken[0].Available() != 50 {
		t.Errorf("recOlder parsed wrong: %+v", ken[0])
	}
	if ken[1].LocalOnly || ken[1].Type != "Overpayment" || ken[1].Available() != 279 {
		t.Errorf("recNewer parsed wrong: %+v", ken[1])
	}
	if len(got["recA"]) != 1 || len(got["recB"]) != 0 {
		t.Errorf("shared row should belong to the first linked member only: %+v", got)
	}
}
