package reconcile

import "testing"

func TestNormalizeState(t *testing.T) {
	cases := []struct {
		in       string
		wantCode string
		wantOk   bool
	}{
		{"FL", "FL", true},
		{"Fl", "FL", true},
		{"fl", "FL", true},
		{"Fla", "FL", true},
		{"Fla.", "FL", true},
		{"Florida", "FL", true},
		{"florida ", "FL", true},
		{" FLORIDA ", "FL", true},
		{"Georgia", "GA", true},
		{"GA", "GA", true},
		{"California", "CA", true},
		{"Calif.", "CA", true},
		{"Texas", "TX", true},
		{"DC", "DC", true},
		{"District of Columbia", "DC", true},
		{"Puerto Rico", "PR", true},
		// unknown
		{"", "", false},
		{"Zanzibar", "Zanzibar", false},
		{"XX", "XX", false},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := NormalizeState(tc.in)
			if got != tc.wantCode || ok != tc.wantOk {
				t.Errorf("NormalizeState(%q) = (%q, %v); want (%q, %v)",
					tc.in, got, ok, tc.wantCode, tc.wantOk)
			}
		})
	}
}
