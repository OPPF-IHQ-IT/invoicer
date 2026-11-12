package duesrun

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// Level says whether a finding stops the run or goes to a person to decide.
type Level string

const (
	Blocker Level = "BLOCKER"
	Warning Level = "WARNING"
)

// Finding is one thing pre-flight wants a person to know.
type Finding struct {
	Level   Level
	Code    string
	Control string // blank for run-wide findings
	Name    string
	Message string
}

// CountByLevel returns how many findings have a level.
func CountByLevel(findings []Finding, level Level) int {
	n := 0
	for _, f := range findings {
		if f.Level == level {
			n++
		}
	}
	return n
}

var requiredLevels = []string{"International", "District", "State", "Local", "Local (Retiree)"}

var prepayNote = regexp.MustCompile(`(?i)prepay|prepaid`)

// Check runs every data check against a snapshot. Setup checks that need no
// data, such as secrets, happen before a snapshot can be gathered.
func Check(cfg *config.Config, s *Snapshot) []Finding {
	var out []Finding
	out = append(out, checkSetup(cfg, s)...)
	out = append(out, checkDuesData(s)...)
	out = append(out, checkMembers(cfg, s)...)
	out = append(out, checkHistory(cfg, s)...)
	out = append(out, checkBalances(cfg, s)...)
	out = append(out, checkTotals(s)...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level == Blocker
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func checkSetup(cfg *config.Config, s *Snapshot) []Finding {
	var out []Finding
	active := map[string]bool{}
	for _, it := range s.Items {
		active[it.ID] = true
	}
	missing := map[string]bool{}
	for _, mp := range s.Plan.MemberPlans {
		if mp.CalcResult == nil || mp.Action == planner.ActionSkip {
			continue
		}
		for _, l := range mp.CalcResult.Lines {
			if d := l.SalesItemLineDetail; d != nil && !active[d.ItemRef.Value] {
				missing[l.Description+" (item "+d.ItemRef.Value+")"] = true
			}
		}
	}
	for _, name := range sortedKeys(missing) {
		out = append(out, Finding{Level: Blocker, Code: "item_missing", Message: "QBO product is missing or inactive: " + name})
	}

	if id := cfg.Invoice.SalesTermID; id != "" {
		found := false
		for _, t := range s.Terms {
			found = found || t.ID == id
		}
		if !found {
			out = append(out, Finding{Level: Blocker, Code: "term_missing", Message: fmt.Sprintf("invoice.sales_term_id %q isn't a QBO term", id)})
		}
	}
	return out
}

func checkDuesData(s *Snapshot) []Finding {
	var out []Finding
	now, before := resolveRates(s.Schedule), resolveRates(s.PriorSchedule)
	for _, level := range requiredLevels {
		if _, ok := now[level]; !ok {
			out = append(out, Finding{Level: Blocker, Code: "schedule_missing", Message: "Dues Schedule has no row for " + level})
		}
	}

	var changed []string
	for _, level := range sortedRateKeys(now) {
		if old, ok := before[level]; ok && old != now[level] {
			changed = append(changed, fmt.Sprintf("%s $%.2f to $%.2f", level, old, now[level]))
		}
	}
	msg := "Dues rates are unchanged from last year. Confirm that's intended."
	if len(changed) > 0 {
		msg = "Dues rates changed from last year: " + strings.Join(changed, "; ") + ". Confirm these are right."
	}
	out = append(out, Finding{Level: Warning, Code: "rates_confirm", Message: msg})

	for _, row := range s.PollRows {
		if math.Abs(row.Available-(row.Earned-row.Spent)) > 0.005 {
			out = append(out, Finding{Level: Warning, Code: "credit_math", Message: fmt.Sprintf("Poll worker row %s: available $%.2f doesn't equal earned minus spent", row.RecordID, row.Available)})
		}
		if row.Available > 0 && len(row.MemberIDs) != 1 {
			out = append(out, Finding{Level: Warning, Code: "credit_link", Message: fmt.Sprintf("Poll worker row %s links to %d members, so its $%.2f may be used wrongly", row.RecordID, len(row.MemberIDs), row.Available)})
		}
	}
	for _, row := range s.CreditRows {
		if row.Available > 0 && len(row.MemberIDs) != 1 {
			out = append(out, Finding{Level: Warning, Code: "credit_link", Message: fmt.Sprintf("Member Credits row %s links to %d members, so its $%.2f may be used wrongly", row.RecordID, len(row.MemberIDs), row.Available)})
		}
	}
	return out
}

func checkMembers(cfg *config.Config, s *Snapshot) []Finding {
	var out []Finding
	seen := map[string]string{}
	pollByMember := pollAvailable(s)

	for _, mp := range s.Plan.MemberPlans {
		m := mp.Member
		add := func(level Level, code, msg string) {
			out = append(out, Finding{Level: level, Code: code, Control: m.ControlNumber, Name: m.Name, Message: msg})
		}

		if m.Email != strings.TrimSpace(m.Email) {
			add(Warning, "email_spaces", "Airtable email has stray spaces")
		}
		switch mp.SkipReason {
		case "missing_email":
			add(Warning, "no_customer", "No email and no QBO customer, so this member won't be billed")
		case "":
		default:
			add(Warning, "skipped", fmt.Sprintf("Won't be billed: %s %s", mp.SkipReason, mp.SkipDetail))
		}

		c := mp.QBOCustomer
		if c == nil {
			continue
		}
		if other, dup := seen[c.ID]; dup {
			add(Blocker, "shared_customer", fmt.Sprintf("Shares QBO customer %s with %s", c.ID, other))
		}
		seen[c.ID] = m.Name

		if c.Email == "" {
			add(Warning, "no_qbo_email", "QBO customer has no email, so nothing can be sent")
		} else if !strings.EqualFold(strings.TrimSpace(c.Email), strings.TrimSpace(m.Email)) {
			add(Warning, "email_mismatch", fmt.Sprintf("Email goes to QBO's %s, but Airtable has %s", c.Email, strings.TrimSpace(m.Email)))
		}
		if !namesAgree(m.Name, c.DisplayName) {
			add(Warning, "name_mismatch", fmt.Sprintf("Airtable name doesn't look like QBO customer %q", c.DisplayName))
		}

		if m.MSPExpired {
			add(Warning, "msp_ended", fmt.Sprintf("Prepaid through %s, so the MSP offset no longer applies and this member now owes dues. A real create unchecks Recent MSP", m.MSPPrepaidThrough))
		} else if code, msg := mspFinding(m.RecentMSP, m.MSPPrepaidThrough, s.FY.Number); code != "" {
			add(Warning, code, msg)
		}
		if mp.CalcResult != nil && mp.CalcResult.Total > 0 && pollByMember[m.RecordID] > 0 && !hasPollLine(cfg, mp) {
			add(Warning, "poll_credit_unused", fmt.Sprintf("Has $%.2f in poll worker credit that can't apply, because local dues are already $0", pollByMember[m.RecordID]))
		}
	}

	byName := map[string][]qbo.Customer{}
	for _, c := range s.Customers {
		byName[nameKey(c.DisplayName)] = append(byName[nameKey(c.DisplayName)], c)
	}
	for _, key := range sortedCustomerKeys(byName) {
		if group := byName[key]; key != "" && len(group) > 1 {
			names := make([]string, len(group))
			for i, c := range group {
				names[i] = fmt.Sprintf("%s (%s)", c.DisplayName, c.ID)
			}
			out = append(out, Finding{Level: Warning, Code: "duplicate_customer", Message: "Possible duplicate QBO customers: " + strings.Join(names, ", ")})
		}
	}
	return out
}

func mspFinding(recent bool, through string, fyNumber int) (string, string) {
	through = strings.TrimSpace(through)
	year := parseFY(through)
	switch {
	case recent && through == "":
		return "msp_unknown", "Recent MSP, but MSP Prepaid Through is blank. Confirm the offset still applies"
	case recent && year == 0:
		return "msp_unknown", fmt.Sprintf("Can't read MSP Prepaid Through %q. Use a value like FY2028", through)
	case !recent && year >= fyNumber:
		return "msp_unchecked", fmt.Sprintf("Prepaid through %s, but Recent MSP isn't checked", through)
	}
	return "", ""
}

func parseFY(s string) int {
	var n int
	fmt.Sscanf(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(s)), "FY"), "%d", &n)
	return n
}

// features are the billing traits a dues document reveals.
type features struct {
	Retiree, MSP, Undergrad, AtLarge, IntlLife, DistrictLife, StateLife, LocalLife, Basileus bool
}

func docFeatures(lines []qbo.InvoiceLine) features {
	var f features
	upper := false
	for _, l := range lines {
		n := l.Description
		if l.SalesItemLineDetail != nil && l.SalesItemLineDetail.ItemRef.Name != "" {
			n = l.SalesItemLineDetail.ItemRef.Name
		}
		f.Retiree = f.Retiree || strings.Contains(n, "Retiree")
		f.MSP = f.MSP || strings.Contains(n, "MSP")
		f.Undergrad = f.Undergrad || strings.Contains(n, "Undergraduate")
		f.IntlLife = f.IntlLife || strings.Contains(n, "International Life")
		f.DistrictLife = f.DistrictLife || strings.Contains(n, "District Life")
		f.StateLife = f.StateLife || strings.Contains(n, "State Life")
		f.LocalLife = f.LocalLife || strings.Contains(n, "Local Life")
		f.Basileus = f.Basileus || strings.Contains(n, "Basileus")
		upper = upper || strings.Contains(n, "International") || strings.Contains(n, "District") || strings.Contains(n, "State")
	}
	f.AtLarge = !upper
	return f
}

// lastYearDoc picks the member's fullest dues document from the prior year.
// Members often have several, and the one with the most dues lines is the
// statement rather than a partial payment.
func lastYearDoc(s *Snapshot) map[string]docSummary {
	best := map[string]docSummary{}
	consider := func(customer, note string, total float64, lines []qbo.InvoiceLine) {
		if strings.HasPrefix(strings.ToLower(note), "voided") {
			return
		}
		n := 0
		for _, l := range lines {
			if isDuesLine(l) {
				n++
			}
		}
		if n > 0 && n > best[customer].duesLines {
			best[customer] = docSummary{total: total, lines: lines, duesLines: n}
		}
	}
	for _, i := range s.PriorInvoices {
		consider(i.CustomerRef.Value, i.PrivateNote, i.TotalAmt, i.Line)
	}
	for _, r := range s.PriorReceipts {
		consider(r.CustomerRef.Value, r.PrivateNote, r.TotalAmt, r.Line)
	}
	return best
}

type docSummary struct {
	total     float64
	lines     []qbo.InvoiceLine
	duesLines int
}

func isDuesLine(l qbo.InvoiceLine) bool {
	if l.SalesItemLineDetail == nil {
		return false
	}
	return strings.Contains(l.SalesItemLineDetail.ItemRef.Name, "Dues") && !strings.Contains(l.SalesItemLineDetail.ItemRef.Name, "Offset")
}

func checkHistory(cfg *config.Config, s *Snapshot) []Finding {
	var out []Finding
	last := lastYearDoc(s)
	nowRates, oldRates := resolveRates(s.Schedule), resolveRates(s.PriorSchedule)
	names := map[string]string{}
	for _, it := range s.Items {
		names[it.ID] = it.Name
	}

	for _, mp := range s.Plan.MemberPlans {
		if mp.CalcResult == nil || mp.QBOCustomer == nil || !creates(mp) {
			continue
		}
		m := mp.Member
		add := func(code, msg string) {
			out = append(out, Finding{Level: Warning, Code: code, Control: m.ControlNumber, Name: m.Name, Message: msg})
		}
		doc, ok := last[mp.QBOCustomer.ID]
		if !ok {
			if mp.CalcResult.Total > 0 {
				add("first_time_billing", "No dues record in QBO last year, so this may be a first invoice. Confirm the status isn't Reclaimable")
			}
			continue
		}

		lines := make([]qbo.InvoiceLine, len(mp.CalcResult.Lines))
		copy(lines, mp.CalcResult.Lines)
		for i, l := range lines {
			if d := l.SalesItemLineDetail; d != nil {
				detail := *d
				detail.ItemRef.Name = names[d.ItemRef.Value]
				lines[i].SalesItemLineDetail = &detail
			}
		}
		was, now := docFeatures(doc.lines), docFeatures(lines)
		for _, c := range []struct {
			code, label string
			was, now    bool
		}{
			{"msp_change", "Recent MSP", was.MSP, now.MSP},
			{"undergrad_change", "Undergraduate rate", was.Undergrad, now.Undergrad},
			{"retiree_change", "Retiree rate", was.Retiree, now.Retiree},
			{"at_large_change", "At Large", was.AtLarge, now.AtLarge},
			{"basileus_change", "Basileus Emeritus", was.Basileus, now.Basileus},
			{"life_change", "International Life", was.IntlLife, now.IntlLife},
			{"life_change", "District Life", was.DistrictLife, now.DistrictLife},
			{"life_change", "State Life", was.StateLife, now.StateLife},
			{"life_change", "Chi Tau Life", was.LocalLife, now.LocalLife},
		} {
			if c.was != c.now {
				verb := "gained"
				if c.was {
					verb = "lost"
				}
				add(c.code, fmt.Sprintf("%s %s since last year ($%.2f last year, $%.2f now)", c.label, verb, doc.total, mp.CalcResult.Total))
			}
		}

		rise := nowRates["Local"] - oldRates["Local"]
		if diff := mp.CalcResult.Total - doc.total; math.Abs(diff) > math.Abs(rise)+0.005 && was == now {
			add("bill_change", fmt.Sprintf("Bill moved $%.2f (from $%.2f to $%.2f), more than the rate change", diff, doc.total, mp.CalcResult.Total))
		}
	}
	return out
}

func checkBalances(cfg *config.Config, s *Snapshot) []Finding {
	var out []Finding
	customers := map[string]qbo.Customer{}
	for _, c := range s.Customers {
		customers[c.ID] = c
	}
	duesItems := DuesItems(cfg)
	billed := map[string]bool{}
	for _, inv := range s.Invoices {
		if inv.TotalAmt > 0 && hasItem(inv.Line, duesItems) {
			billed[inv.CustomerRef.Value] = true
		}
	}
	for _, r := range s.Receipts {
		if hasItem(r.Line, duesItems) {
			billed[r.CustomerRef.Value] = true
		}
	}
	credited := map[string]bool{}
	for _, rows := range s.MemberCredits {
		for _, c := range rows {
			credited[c.MemberID] = true
		}
	}
	unapplied := map[string]float64{}
	for _, p := range s.RecentPayments {
		unapplied[p.CustomerRef.Value] += p.UnappliedAmt
	}
	prepaid := map[string]string{}
	for _, r := range s.RecentReceipts {
		text := r.PrivateNote
		for _, l := range r.Line {
			text += " " + l.Description
		}
		if prepayNote.MatchString(text) {
			prepaid[r.CustomerRef.Value] = r.DocNumber
		}
	}

	for _, mp := range s.Plan.MemberPlans {
		if mp.QBOCustomer == nil || !creates(mp) {
			continue
		}
		m, id := mp.Member, mp.QBOCustomer.ID
		add := func(level Level, code, msg string) {
			out = append(out, Finding{Level: level, Code: code, Control: m.ControlNumber, Name: m.Name, Message: msg})
		}
		if billed[id] {
			add(Blocker, "already_billed", "Already has a dues document for this fiscal year. Remove it or skip this member")
		}
		switch c := customers[id]; {
		case c.Balance < 0 && !credited[m.RecordID]:
			add(Warning, "qbo_credit", fmt.Sprintf("Has a $%.2f credit in QBO but no Member Credits row, so the invoice won't reflect it", -c.Balance))
		case c.Balance > 0:
			add(Warning, "open_balance", fmt.Sprintf("Already owes $%.2f from earlier documents", c.Balance))
		}
		if amt := unapplied[id]; amt > 0 {
			add(Warning, "unapplied_payment", fmt.Sprintf("Has $%.2f in recent unapplied payments", amt))
		}
		if doc, ok := prepaid[id]; ok {
			add(Warning, "possible_prepayment", fmt.Sprintf("Receipt %s looks like a prepayment. Confirm it's in Member Credits", doc))
		}
	}
	return out
}

// checkTotals compares every planned total with the separate recompute.
func checkTotals(s *Snapshot) []Finding {
	var out []Finding
	r := resolveRates(s.Schedule)
	poll := pollAvailable(s)
	late := s.FY.IsLate(s.AsOf)
	for _, mp := range s.Plan.MemberPlans {
		if mp.CalcResult == nil || !creates(mp) {
			continue
		}
		want := recomputeTotal(mp.Member, r, late, poll[mp.Member.RecordID], s.MemberCredits[mp.Member.RecordID])
		if math.Abs(want-mp.CalcResult.Total) > 0.005 {
			out = append(out, Finding{Level: Blocker, Code: "total_mismatch", Control: mp.Member.ControlNumber, Name: mp.Member.Name,
				Message: fmt.Sprintf("Plan says $%.2f, but the separate recompute says $%.2f", mp.CalcResult.Total, want)})
		}
	}
	return out
}

func creates(mp planner.MemberPlan) bool {
	return mp.Action == planner.ActionCreate || mp.Action == planner.ActionCreateReceipt
}

func pollAvailable(s *Snapshot) map[string]float64 {
	totals := map[string]float64{}
	for _, row := range s.PollRows {
		if row.Available <= 0 {
			continue
		}
		for _, id := range row.MemberIDs {
			totals[id] += row.Available
		}
	}
	return totals
}

func hasPollLine(cfg *config.Config, mp planner.MemberPlan) bool {
	for _, l := range mp.CalcResult.Lines {
		if d := l.SalesItemLineDetail; d != nil && d.ItemRef.Value == cfg.QBOItems.PollWorkerCredit {
			return true
		}
	}
	return false
}

func hasItem(lines []qbo.InvoiceLine, items map[string]bool) bool {
	for _, l := range lines {
		if d := l.SalesItemLineDetail; d != nil && items[d.ItemRef.Value] {
			return true
		}
	}
	return false
}

var nameSuffix = regexp.MustCompile(`(?i)\b(mr|mrs|dr|rev|bro|atty|jr|sr|ii|iii|iv|esq|phd|md|edd|mba|jd|pharmd)\b`)

func nameWords(s string) []string {
	s = strings.ToLower(strings.NewReplacer(".", "", ",", " ", "\"", " ").Replace(s))
	return strings.Fields(nameSuffix.ReplaceAllString(s, " "))
}

// namesAgree is deliberately loose: nicknames and middle names are common, so
// it only flags names sharing no words at all, or only a shared first name.
func namesAgree(a, b string) bool {
	wa, wb := nameWords(a), nameWords(b)
	if len(wa) == 0 || len(wb) == 0 {
		return true
	}
	return wa[len(wa)-1] == wb[len(wb)-1]
}

func nameKey(s string) string {
	w := nameWords(s)
	if len(w) < 2 {
		return ""
	}
	first := w[0]
	if len(first) > 4 {
		first = first[:4]
	}
	return first + " " + w[len(w)-1]
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedRateKeys(m rates) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedCustomerKeys(m map[string][]qbo.Customer) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// DefaultInvoiceDate is the date a run uses unless told otherwise: the first
// day of the fiscal year.
func DefaultInvoiceDate(periodStart time.Time) time.Time {
	return time.Date(periodStart.Year(), periodStart.Month(), periodStart.Day(), 0, 0, 0, 0, time.Local)
}
