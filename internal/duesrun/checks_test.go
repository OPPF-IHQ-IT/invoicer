package duesrun

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/invoicecalc"
	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

func codes(findings []Finding) string {
	var c []string
	for _, f := range findings {
		c = append(c, f.Code)
	}
	return strings.Join(c, ",")
}

func hasCode(findings []Finding, code string) bool {
	for _, f := range findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

// The recompute must agree with the real calculator across every combination
// of flags, otherwise the check would cry wolf or miss real errors.
func TestRecomputeAgreesWithCalculator(t *testing.T) {
	fy := testFY(t)
	schedule := append(append([]airtable.DuesScheduleRow{}, testSchedule...),
		airtable.DuesScheduleRow{FiscalYear: "ALL", Level: "International (Undergraduate)", Amount: 95},
		airtable.DuesScheduleRow{FiscalYear: "ALL", Level: "International Reinstatement", Amount: 3},
		airtable.DuesScheduleRow{FiscalYear: "ALL", Level: "International Late Fee", Amount: 5},
		airtable.DuesScheduleRow{FiscalYear: "ALL", Level: "District Late Fee", Amount: 3},
		airtable.DuesScheduleRow{FiscalYear: "ALL", Level: "State Late Fee", Amount: 5},
		airtable.DuesScheduleRow{FiscalYear: "ALL", Level: "Local Late Fee", Amount: 20},
	)
	items := config.QBOItemsConfig{
		International: "i", District: "d", State: "s", Local: "l", LocalRetiree: "lr", InternationalUndergrad: "iu",
		InternationalLifeMember: "a", DistrictLifeMember: "b", StateLifeMember: "c", LocalLifeMember: "e", LocalLifeMemberRetiree: "f",
		BasileusEmeritusOffset: "g", BasileusEmeritusOffsetRetiree: "h", InternationalMSP: "j", DistrictMSP: "k", StateMSP: "m",
		LocalMSP: "n", LocalMSPRetiree: "o", PollWorkerCredit: "p", PollWorkerCreditUnit: 50, InternationalReinstatement: "q", MemberCredit: "r",
		InternationalLateFee: "t", DistrictLateFee: "u", StateLateFee: "v", LocalLateFee: "w",
	}
	r := resolveRates(schedule)
	credits := [][]airtable.MemberCredit{
		nil,
		{{Earned: 30}},
		{{Earned: 5000}},
		{{Earned: 40, LocalOnly: true}, {Earned: 20}},
		{{Earned: 500, LocalOnly: true}, {Earned: 100, MaxPerYear: 25}},
	}
	checked := 0
	for bits := 0; bits < 1<<10; bits++ {
		on := func(i int) bool { return bits&(1<<i) != 0 }
		m := airtable.Member{
			IntlLife: on(0), DistrictLife: on(1), StateLife: on(2), LocalLife: on(3), BasileusEmeritus: on(4),
			Retired: on(5), RecentMSP: on(6), ChapterInitiate: on(7), AtLarge: on(8), UndergradRate: on(9),
		}
		for _, reclaimable := range []bool{false, true} {
			m.Reclaimable = reclaimable
			for _, late := range []bool{false, true} {
				asOf := fy.PeriodStart
				if late {
					asOf = fy.LateFeeStartsOn
				}
				for _, poll := range []float64{0, 50, 500} {
					for _, c := range credits {
						want := invoicecalc.Calculate(invoicecalc.CalculationInput{FY: fy, AsOf: asOf, Member: m, PollWorkerCredit: poll, MemberCredits: c, Schedule: schedule, Items: items})
						if want.SkipReason != "" {
							t.Fatalf("skip %s for %+v", want.SkipReason, m)
						}
						got := recomputeTotal(m, r, late, poll, c)
						if math.Abs(got-want.Total) > 0.005 {
							t.Fatalf("member %+v late=%v poll=%v credits=%+v: recompute %.2f, calculator %.2f", m, late, poll, c, got, want.Total)
						}
						checked++
					}
				}
			}
		}
	}
	t.Logf("%d combinations agree", checked)
}

func TestMSPFinding(t *testing.T) {
	for _, tc := range []struct {
		recent  bool
		through string
		want    string
	}{
		{true, "", "msp_unknown"},
		{true, "next year", "msp_unknown"},
		{true, "FY2028", ""},
		{true, "FY2029", ""},
		{false, "FY2028", "msp_unchecked"},
		{false, "FY2027", ""},
		{false, "", ""},
	} {
		if got, _ := mspFinding(tc.recent, tc.through, 2028); got != tc.want {
			t.Errorf("recent=%v through=%q: got %q, want %q", tc.recent, tc.through, got, tc.want)
		}
	}
}

func TestNamesAgree(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"Gregory Sample", "Greg Sample", true},
		{"William Sample, Jr.", "Will Sample", true},
		{"Jerome Sample Ed.D.", "E. Jerome Sample, Ed.D.", true},
		{"Sample Brother", "Different Person", false},
	} {
		if got := namesAgree(tc.a, tc.b); got != tc.want {
			t.Errorf("%q vs %q = %v", tc.a, tc.b, got)
		}
	}
}

func TestMemberAndBalanceFlags(t *testing.T) {
	fy := testFY(t)
	plan := standardPlan(fy, 408)
	mp := &plan.MemberPlans[0]
	mp.Member.Email = "old@b.c "
	mp.QBOCustomer.Email = "new@b.c"
	mp.Member.RecentMSP = false
	mp.Member.MSPPrepaidThrough = "FY2029"
	second := plan.MemberPlans[0]
	second.Member.ControlNumber, second.Member.RecordID, second.Member.Name = "002", "rec2", "Sample Twin"
	plan.MemberPlans = append(plan.MemberPlans, second)

	s := snapshot(fy, plan)
	s.Customers = []qbo.Customer{{ID: "1", DisplayName: "Sample Brother", Balance: -5}, {ID: "2", DisplayName: "Mr Sample Brother"}}
	s.PollRows = []airtable.CreditRow{{Earned: 50, Spent: 0, Available: 50, MemberIDs: []string{"rec1", "rec2"}}}
	s.Invoices = []qbo.Invoice{{CustomerRef: qbo.CustomerRef{Value: "1"}, TotalAmt: 408, Line: []qbo.InvoiceLine{line("79", "Local Dues", 248)}}}
	s.RecentPayments = []qbo.Payment{{CustomerRef: qbo.CustomerRef{Value: "1"}, UnappliedAmt: 20}}

	got := Check(testConfig(), s)
	for _, code := range []string{"email_spaces", "email_mismatch", "shared_customer", "msp_unchecked", "qbo_credit", "already_billed", "unapplied_payment", "credit_link", "duplicate_customer", "first_time_billing"} {
		if !hasCode(got, code) {
			t.Errorf("missing %s in %s", code, codes(got))
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Level == Warning && got[i].Level == Blocker {
			t.Fatal("blockers should come first")
		}
	}
}

func TestSetupAndScheduleBlockers(t *testing.T) {
	fy := testFY(t)
	s := snapshot(fy, standardPlan(fy, 408))
	s.Items = s.Items[:1]
	s.Schedule = s.Schedule[:2]
	cfg := testConfig()
	cfg.Invoice.SalesTermID = "1"
	got := Check(cfg, s)
	for _, code := range []string{"item_missing", "schedule_missing", "term_missing"} {
		if !hasCode(got, code) {
			t.Errorf("missing %s in %s", code, codes(got))
		}
	}
}

func TestHistoryFlagsLostMSP(t *testing.T) {
	fy := testFY(t)
	s := snapshot(fy, standardPlan(fy, 408))
	s.Items = []qbo.Item{{ID: "65", Name: "International Membership Dues"}, {ID: "71", Name: "District Membership Dues"}, {ID: "75", Name: "State Membership Dues"}, {ID: "79", Name: "Local Membership Dues"}, {ID: "80"}}
	named := func(name string, amount float64) qbo.InvoiceLine {
		l := line("x", name, amount)
		l.SalesItemLineDetail.ItemRef.Name = name
		return l
	}
	s.PriorReceipts = []qbo.SalesReceipt{{CustomerRef: qbo.CustomerRef{Value: "1"}, Line: []qbo.InvoiceLine{
		named("International Membership Dues", 125), named("International Recent MSP Membership Dues Offset", -125),
		named("District Membership Dues", 20), named("State Membership Dues", 15), named("Local Membership Dues", 198),
	}}}
	got := Check(testConfig(), s)
	if !hasCode(got, "msp_change") || hasCode(got, "first_time_billing") {
		t.Fatalf("got %s", codes(got))
	}
}

func TestVerifyCreatedSpotsDifferences(t *testing.T) {
	fy := testFY(t)
	plan := standardPlan(fy, 408)
	plan.MemberPlans[0].ExistingInvoice = &qbo.Invoice{ID: "9"}
	good := plan.MemberPlans[0].CalcResult.Lines
	r := fakeReader{inv: &qbo.Invoice{TotalAmt: 408, BillEmail: "a@b.c", Line: good}}
	if p := VerifyCreated(context.Background(), r, plan); len(p) != 0 {
		t.Fatalf("clean invoice flagged: %v", p)
	}
	r.inv = &qbo.Invoice{TotalAmt: 400, Line: good[:3]}
	if p := VerifyCreated(context.Background(), r, plan); len(p) != 3 {
		t.Fatalf("got %v", p)
	}
	plan.MemberPlans[0].SkipDetail = "status update failed"
	r.inv = &qbo.Invoice{TotalAmt: 408, BillEmail: "a@b.c", Line: good}
	if p := VerifyCreated(context.Background(), r, plan); len(p) != 1 {
		t.Fatalf("partial failure hidden: %v", p)
	}
}

type fakeReader struct {
	inv *qbo.Invoice
	sr  *qbo.SalesReceipt
}

func (f fakeReader) GetInvoice(context.Context, string) (*qbo.Invoice, error) { return f.inv, nil }
func (f fakeReader) GetSalesReceipt(context.Context, string) (*qbo.SalesReceipt, error) {
	return f.sr, nil
}

func TestVerifySentIgnoresNothing(t *testing.T) {
	fy := testFY(t)
	plan := sendable(fy)
	r := fakeReader{inv: &qbo.Invoice{EmailStatus: "NotSet", DocNumber: "1"}}
	if p := VerifySent(context.Background(), r, plan); len(p) != 1 {
		t.Fatalf("got %v", p)
	}
}

func TestBuildStatus(t *testing.T) {
	dues := map[string]bool{"79": true}
	due := []qbo.InvoiceLine{line("79", "Local Dues", 248)}
	st := BuildStatus([]qbo.Invoice{
		{TotalAmt: 248, Balance: 248, EmailStatus: "EmailSent", Line: due},
		{TotalAmt: 248, Balance: 0, EmailStatus: "EmailSent", Line: due},
		{TotalAmt: 248, Balance: 248, EmailStatus: "EmailSent", DeliveryError: "Undeliverable", Line: due},
		{TotalAmt: 0, PrivateNote: "Voided - x", Line: due},
		{TotalAmt: 500, Balance: 500, Line: []qbo.InvoiceLine{line("9", "Facilities", 500)}},
	}, []qbo.SalesReceipt{{Line: due}, {TotalAmt: 100, Line: due}}, dues, fiscal.Year{})
	want := fmt.Sprintf("%d %d %d %.0f %.0f %d %d %d", 3, 1, 2, 744.0, 496.0, 1, 1, 1)
	got := fmt.Sprintf("%d %d %d %.0f %.0f %d %d %d", st.Invoices, st.Paid, st.Open, st.Billed, st.Outstanding, st.Voided, st.Receipts, len(st.Bounces))
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

var _ = planner.ActionSend
