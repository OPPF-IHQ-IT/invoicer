package reconcile

import "strings"

// usStates maps each USPS two-letter code to its full state/territory name.
var usStates = map[string]string{
	"AL": "Alabama", "AK": "Alaska", "AZ": "Arizona", "AR": "Arkansas",
	"CA": "California", "CO": "Colorado", "CT": "Connecticut", "DE": "Delaware",
	"DC": "District of Columbia", "FL": "Florida", "GA": "Georgia", "HI": "Hawaii",
	"ID": "Idaho", "IL": "Illinois", "IN": "Indiana", "IA": "Iowa",
	"KS": "Kansas", "KY": "Kentucky", "LA": "Louisiana", "ME": "Maine",
	"MD": "Maryland", "MA": "Massachusetts", "MI": "Michigan", "MN": "Minnesota",
	"MS": "Mississippi", "MO": "Missouri", "MT": "Montana", "NE": "Nebraska",
	"NV": "Nevada", "NH": "New Hampshire", "NJ": "New Jersey", "NM": "New Mexico",
	"NY": "New York", "NC": "North Carolina", "ND": "North Dakota", "OH": "Ohio",
	"OK": "Oklahoma", "OR": "Oregon", "PA": "Pennsylvania", "RI": "Rhode Island",
	"SC": "South Carolina", "SD": "South Dakota", "TN": "Tennessee", "TX": "Texas",
	"UT": "Utah", "VT": "Vermont", "VA": "Virginia", "WA": "Washington",
	"WV": "West Virginia", "WI": "Wisconsin", "WY": "Wyoming",
	"AS": "American Samoa", "GU": "Guam", "MP": "Northern Mariana Islands",
	"PR": "Puerto Rico", "VI": "Virgin Islands",
}

// extraStateAliases maps common non-standard spellings/abbreviations to the
// USPS two-letter code. Keys are already normalised (lower-case, trimmed,
// trailing "." removed).
var extraStateAliases = map[string]string{
	"fla":   "FL",
	"flor":  "FL",
	"cali":  "CA",
	"calif": "CA",
	"mass":  "MA",
	"penn":  "PA",
	"penna": "PA",
	"tex":   "TX",
	"wash":  "WA",
	"ariz":  "AZ",
}

// NormalizeState returns the USPS two-letter code for an input like "FL", "Fla",
// "Florida", "florida ". The second return value is false when the input can't
// be resolved — callers should pass the original value through and flag it.
func NormalizeState(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	key := strings.ToLower(strings.TrimSuffix(s, "."))
	key = strings.TrimSpace(key)

	// Direct two-letter match.
	if len(key) == 2 {
		up := strings.ToUpper(key)
		if _, ok := usStates[up]; ok {
			return up, true
		}
	}

	// Full-name match.
	for code, name := range usStates {
		if strings.EqualFold(name, key) {
			return code, true
		}
	}

	// Alias match.
	if code, ok := extraStateAliases[key]; ok {
		return code, true
	}

	return s, false
}
