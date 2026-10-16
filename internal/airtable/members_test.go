package airtable

import "testing"

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
