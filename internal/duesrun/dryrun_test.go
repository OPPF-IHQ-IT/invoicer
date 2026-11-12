package duesrun

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
)

func dryFlow(s *Snapshot, dir, input string, out *bytes.Buffer) *Flow {
	return &Flow{
		Cfg: testConfig(), Dir: dir, In: strings.NewReader(input), Out: out, DryRun: true,
		Ops: &DryRunOps{GatherFn: func(context.Context) (*Snapshot, error) { return s, nil }},
		Now: func() time.Time { return time.Date(2027, 11, 2, 0, 0, 0, 0, time.UTC) },
	}
}

func TestDryRunWalksEveryStageWithoutTouchingThePlan(t *testing.T) {
	fy := testFY(t)
	plan := standardPlan(fy, 408)
	s := snapshot(fy, plan)
	var out bytes.Buffer
	input := reviewPhrase(testConfig(), s) + "\ncreate 1 + 0\nsend 1 + 0\n"
	if err := dryFlow(s, t.TempDir(), input, &out).Run(context.Background()); err != nil {
		t.Fatalf("err = %v\n%s", err, out.String())
	}
	for _, want := range []string{"DRY RUN", "2. Review (dry run)", "3. Create (dry run)", "5. Send (dry run)", "6. Verify send (dry run)", "Simulated.", "Nothing was created, emailed, or changed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in:\n%s", want, out.String())
		}
	}
	mp := plan.MemberPlans[0]
	if mp.Action != planner.ActionCreate || mp.ExistingInvoice != nil {
		t.Fatalf("real plan was changed: %+v", mp.Action)
	}
}

func TestDryRunNeedsTheSameTypedPhrases(t *testing.T) {
	fy := testFY(t)
	s := snapshot(fy, standardPlan(fy, 408))
	var out bytes.Buffer
	if err := dryFlow(s, t.TempDir(), "nope\n", &out).Run(context.Background()); err != ErrStopped {
		t.Fatalf("err = %v", err)
	}
}

// Dry-run approvals live in their own folder, chosen by the CLI. This pins the
// other half: a real run must not trust a state file with a different plan.
func TestApprovalFromAnotherPlanIsNotReused(t *testing.T) {
	fy := testFY(t)
	dir := t.TempDir()
	if err := SaveReview(dir, &ReviewState{Approved: true, Fingerprint: "other"}); err != nil {
		t.Fatal(err)
	}
	s := snapshot(fy, standardPlan(fy, 408))
	ops := &fakeOps{snaps: []*Snapshot{s}}
	if _, err := runFlow(t, ops, dir, "create 1 + 0\n"); err != ErrStopped {
		t.Fatalf("err = %v", err)
	}
	if ops.created != 0 {
		t.Fatal("trusted an approval for a different plan")
	}
	if _, err := os.Stat(filepath.Join(dir, "review.csv")); err != nil {
		t.Fatal("review file should be rebuilt")
	}
}
