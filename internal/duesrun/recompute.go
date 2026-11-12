package duesrun

import (
	"strings"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
)

// rates is the resolved dues schedule: fiscal-year rows win over ALL rows.
type rates map[string]float64

func resolveRates(rows []airtable.DuesScheduleRow) rates {
	r := rates{}
	specific := map[string]bool{}
	for _, row := range rows {
		isAll := strings.EqualFold(strings.TrimSpace(row.FiscalYear), "ALL")
		if isAll && specific[row.Level] {
			continue
		}
		r[row.Level] = row.Amount
		if !isAll {
			specific[row.Level] = true
		}
	}
	return r
}

// recomputeTotal works out what a member owes from first principles. It's kept
// separate from invoicecalc on purpose: it deals only in dollars, with no line
// items or QBO products, so a bug in one is unlikely to be repeated in the other.
func recomputeTotal(m airtable.Member, r rates, late bool, poll float64, credits []airtable.MemberCredit) float64 {
	var upper float64
	if !m.AtLarge {
		intl := r["International"]
		if m.UndergradRate {
			intl = r["International (Undergraduate)"]
		}
		if !m.IntlLife && !m.RecentMSP {
			upper += intl
		}
		if !m.DistrictLife && !m.RecentMSP {
			upper += r["District"]
		}
		if !m.StateLife && !m.RecentMSP {
			upper += r["State"]
		}
		if m.Reclaimable {
			upper += r["International Reinstatement"]
		}
		if late {
			upper += r["International Late Fee"] + r["District Late Fee"] + r["State Late Fee"]
		}
	}

	local := r["Local"]
	if m.Retired {
		local = r["Local (Retiree)"]
	}
	if m.BasileusEmeritus || m.LocalLife || (m.RecentMSP && m.ChapterInitiate) {
		local = 0
	} else if poll > 0 {
		local -= minFloat(poll, local)
	}
	if late {
		local += r["Local Late Fee"]
	}

	total := upper + local
	for _, localOnly := range []bool{true, false} {
		for _, c := range credits {
			if c.LocalOnly != localOnly {
				continue
			}
			used := minFloat(c.Available(), total)
			if localOnly {
				used = minFloat(used, local)
			}
			if c.MaxPerYear > 0 {
				used = minFloat(used, c.MaxPerYear)
			}
			if used <= 0 {
				continue
			}
			total -= used
			if localOnly {
				local -= used
			}
		}
	}
	return total
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
