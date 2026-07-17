package config

import "testing"

func TestItemForDesignation(t *testing.T) {
	c := CampaignConfig{Designations: map[string]string{
		"Facility Utilities (Water/Lights)": "19",
		"Facility Internet (Fiber)":         "42",
	}}

	cases := []struct {
		name       string
		in         string
		wantID     string
		wantOK     bool
	}{
		{"exact water", "Facility Utilities (Water/Lights)", "19", true},
		{"exact internet", "Facility Internet (Fiber)", "42", true},
		{"case insensitive", "facility internet (fiber)", "42", true},
		{"extra spaces collapse", "Facility   Internet  (Fiber)", "42", true},
		{"unmapped", "Facility Landscaping", "", false},
		{"empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, ok := c.ItemForDesignation(tc.in)
			if ok != tc.wantOK || id != tc.wantID {
				t.Errorf("ItemForDesignation(%q): got (%q, %v), want (%q, %v)", tc.in, id, ok, tc.wantID, tc.wantOK)
			}
		})
	}
}

func TestItemForDesignationEmptyMap(t *testing.T) {
	var c CampaignConfig // nil Designations
	if id, ok := c.ItemForDesignation("anything"); ok || id != "" {
		t.Errorf("nil map: got (%q, %v), want (\"\", false)", id, ok)
	}
}
