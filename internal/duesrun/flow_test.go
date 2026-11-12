package duesrun

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/invoicecalc"
	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

var testSchedule = []airtable.DuesScheduleRow{
	{FiscalYear: "ALL", Level: "International", Amount: 125},
	{FiscalYear: "ALL", Level: "District", Amount: 20},
	{FiscalYear: "ALL", Level: "State", Amount: 15},
	{FiscalYear: "ALL", Level: "Local", Amount: 198},
	{FiscalYear: "2027-2028", Level: "Local", Amount: 248},
	{FiscalYear: "ALL", Level: "Local (Retiree)", Amount: 148},
}

func testFY(t *testing.T) fiscal.Year {
	t.Helper()
	fy, err := fiscal.Parse("FY2028")
	if err != nil {
		t.Fatal(err)
	}
	return fy
}

func testConfig() *config.Config {
	cfg := &config.Config{}
	cfg.QBOItems.International, cfg.QBOItems.District, cfg.QBOItems.State = "65", "71", "75"
	cfg.QBOItems.Local, cfg.QBOItems.LocalRetiree = "79", "80"
	cfg.QBOItems.PollWorkerCredit = "97"
	return cfg
}

func line(item, desc string, amount float64) qbo.InvoiceLine {
	return qbo.InvoiceLine{Amount: amount, Description: desc, SalesItemLineDetail: &qbo.SalesItemLineDetail{ItemRef: qbo.ItemRef{Value: item}, Qty: 1, UnitPrice: amount}}
}

func standardPlan(fy fiscal.Year, total float64) *planner.Plan {
	m := airtable.Member{RecordID: "rec1", ControlNumber: "001", Name: "Sample Brother", Email: "a@b.c"}
	return &planner.Plan{FY: fy, AsOf: fy.PeriodStart, MemberPlans: []planner.MemberPlan{{
		Member:      m,
		QBOCustomer: &qbo.Customer{ID: "1", DisplayName: "Sample Brother", Email: "a@b.c"},
		Action:      planner.ActionCreate,
		CalcResult: &invoicecalc.CalculationResult{Total: total, LineHash: "h", Lines: []qbo.InvoiceLine{
			line("65", "International Dues", 125), line("71", "District Dues", 20), line("75", "State Dues", 15), line("79", "Local Dues", total-160),
		}},
	}}}
}

type fakeOps struct {
	t         *testing.T
	snaps     []*Snapshot // one per Gather; the last repeats
	gathers   int
	sendPlan  *planner.Plan
	created   int
	sent      int
	verifyBad []string
	sentBad   []string
}

func (f *fakeOps) Gather(context.Context) (*Snapshot, error) {
	i := f.gathers
	if i >= len(f.snaps) {
		i = len(f.snaps) - 1
	}
	f.gathers++
	return f.snaps[i], nil
}
func (f *fakeOps) Create(_ context.Context, p *planner.Plan) (*planner.Plan, error) {
	f.created++
	return p, nil
}
func (f *fakeOps) VerifyCreated(context.Context, *planner.Plan) []string { return f.verifyBad }
func (f *fakeOps) SendPlan(context.Context) (*planner.Plan, error) {
	if f.sendPlan != nil {
		return f.sendPlan, nil
	}
	return &planner.Plan{}, nil
}
func (f *fakeOps) Send(_ context.Context, p *planner.Plan) (*planner.Plan, error) {
	f.sent++
	return p, nil
}
func (f *fakeOps) VerifySent(context.Context, *planner.Plan) []string { return f.sentBad }

func snapshot(fy fiscal.Year, plan *planner.Plan) *Snapshot {
	return &Snapshot{
		FY: fy, AsOf: fy.PeriodStart, Env: "production", Company: "Test Co", Plan: plan,
		Schedule: testSchedule,
		Items:    []qbo.Item{{ID: "65"}, {ID: "71"}, {ID: "75"}, {ID: "79"}, {ID: "80"}},
	}
}

func sendable(fy fiscal.Year) *planner.Plan {
	return &planner.Plan{FY: fy, AsOf: fy.PeriodStart, MemberPlans: []planner.MemberPlan{{
		Member: airtable.Member{ControlNumber: "001"}, Action: planner.ActionSend,
		ExistingInvoice: &qbo.Invoice{ID: "9", TotalAmt: 408},
	}}}
}

func runFlow(t *testing.T, ops *fakeOps, dir, input string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	f := &Flow{Cfg: testConfig(), Ops: ops, Dir: dir, In: strings.NewReader(input), Out: &out,
		Now: func() time.Time { return time.Date(2027, 11, 2, 0, 0, 0, 0, time.UTC) }}
	err := f.Run(context.Background())
	return out.String(), err
}

// warnings is how many warnings the standard plan raises, for the typed phrase.
func reviewPhrase(cfg *config.Config, s *Snapshot) string {
	return "reviewed " + itoa(CountByLevel(Check(cfg, s), Warning))
}

func itoa(n int) string {
	return strings.TrimSpace(strings.Join([]string{string(rune('0' + n))}, ""))
}

func TestWrongConfirmationNeverWrites(t *testing.T) {
	fy := testFY(t)
	for name, input := range map[string]string{
		"empty":     "",
		"enter":     "\n",
		"yes":       "yes\n",
		"bad count": "reviewed 99\n",
	} {
		t.Run(name, func(t *testing.T) {
			ops := &fakeOps{snaps: []*Snapshot{snapshot(fy, standardPlan(fy, 408))}}
			_, err := runFlow(t, ops, t.TempDir(), input)
			if !errors.Is(err, ErrStopped) {
				t.Fatalf("err = %v, want ErrStopped", err)
			}
			if ops.created != 0 || ops.sent != 0 {
				t.Fatalf("created %d, sent %d", ops.created, ops.sent)
			}
		})
	}
}

func TestCreateNeedsItsOwnConfirmation(t *testing.T) {
	fy := testFY(t)
	s := snapshot(fy, standardPlan(fy, 408))
	ops := &fakeOps{snaps: []*Snapshot{s}}
	input := reviewPhrase(testConfig(), s) + "\nnope\n"
	if _, err := runFlow(t, ops, t.TempDir(), input); !errors.Is(err, ErrStopped) {
		t.Fatalf("err = %v", err)
	}
	if ops.created != 0 {
		t.Fatal("created without the create phrase")
	}
}

func TestFullRun(t *testing.T) {
	fy := testFY(t)
	s := snapshot(fy, standardPlan(fy, 408))
	ops := &fakeOps{snaps: []*Snapshot{s}, sendPlan: sendable(fy)}
	input := reviewPhrase(testConfig(), s) + "\ncreate 1 + 0\nsend 1 + 0\n"
	out, err := runFlow(t, ops, t.TempDir(), input)
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if ops.created != 1 || ops.sent != 1 {
		t.Fatalf("created %d, sent %d", ops.created, ops.sent)
	}
}

func TestPlanChangeAfterReviewStopsCreate(t *testing.T) {
	fy := testFY(t)
	s := snapshot(fy, standardPlan(fy, 408))
	ops := &fakeOps{snaps: []*Snapshot{s, snapshot(fy, standardPlan(fy, 500))}}
	ops.snaps[1].Plan.MemberPlans[0].CalcResult.LineHash = "changed"
	input := reviewPhrase(testConfig(), s) + "\ncreate 1 + 0\n"
	out, err := runFlow(t, ops, t.TempDir(), input)
	if !errors.Is(err, ErrStopped) {
		t.Fatalf("err = %v", err)
	}
	if ops.created != 0 {
		t.Fatal("created a plan nobody reviewed")
	}
	if !strings.Contains(out, "changed since you reviewed") {
		t.Fatalf("output: %s", out)
	}
}

func TestResumeSkipsReviewOfTheSamePlan(t *testing.T) {
	fy := testFY(t)
	s := snapshot(fy, standardPlan(fy, 408))
	dir := t.TempDir()
	ops := &fakeOps{snaps: []*Snapshot{s}}
	if _, err := runFlow(t, ops, dir, reviewPhrase(testConfig(), s)+"\nstop\n"); !errors.Is(err, ErrStopped) {
		t.Fatalf("err = %v", err)
	}
	ops2 := &fakeOps{snaps: []*Snapshot{s}}
	if _, err := runFlow(t, ops2, dir, "create 1 + 0\n"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if ops2.created != 1 {
		t.Fatal("resume didn't create")
	}
}

func TestBlockerStopsBeforeReview(t *testing.T) {
	fy := testFY(t)
	s := snapshot(fy, standardPlan(fy, 999)) // recompute will disagree
	ops := &fakeOps{snaps: []*Snapshot{s}}
	out, err := runFlow(t, ops, t.TempDir(), "reviewed 0\ncreate 1 + 0\n")
	if !errors.Is(err, ErrStopped) || ops.created != 0 {
		t.Fatalf("err = %v, created %d", err, ops.created)
	}
	if !strings.Contains(out, "separate recompute") {
		t.Fatalf("output: %s", out)
	}
}

func TestVerifyProblemsStopBeforeSend(t *testing.T) {
	fy := testFY(t)
	s := snapshot(fy, standardPlan(fy, 408))
	ops := &fakeOps{snaps: []*Snapshot{s}, sendPlan: sendable(fy), verifyBad: []string{"001: bad"}}
	input := reviewPhrase(testConfig(), s) + "\ncreate 1 + 0\nsend 1 + 0\n"
	if _, err := runFlow(t, ops, t.TempDir(), input); !errors.Is(err, ErrStopped) {
		t.Fatalf("err = %v", err)
	}
	if ops.created != 1 || ops.sent != 0 {
		t.Fatalf("created %d, sent %d", ops.created, ops.sent)
	}
}

func TestReviewFilesArePrivateAndNeutralizeFormulas(t *testing.T) {
	fy := testFY(t)
	plan := standardPlan(fy, 408)
	plan.MemberPlans[0].Member.Name = "=HYPERLINK(1)"
	s := snapshot(fy, plan)
	dir := t.TempDir()
	path, err := WriteReviewFile(dir, s, Check(testConfig(), s))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
	data, _ := os.ReadFile(filepath.Join(dir, "review.csv"))
	if strings.Contains(string(data), ",=HYPERLINK") {
		t.Fatal("formula not neutralized")
	}
}
