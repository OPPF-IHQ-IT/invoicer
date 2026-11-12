package duesrun

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/planner"
)

// Ops is everything the flow does to the outside world. The real version talks
// to QBO and Airtable; tests use a fake so no stage can touch live data.
type Ops interface {
	Gather(ctx context.Context) (*Snapshot, error)
	Create(ctx context.Context, plan *planner.Plan) (*planner.Plan, error)
	VerifyCreated(ctx context.Context, plan *planner.Plan) []string
	SendPlan(ctx context.Context) (*planner.Plan, error)
	Send(ctx context.Context, plan *planner.Plan) (*planner.Plan, error)
	VerifySent(ctx context.Context, plan *planner.Plan) []string
}

// Flow is one guided run.
type Flow struct {
	Cfg *config.Config
	Ops Ops
	Dir string // where review files live
	In  io.Reader
	Out io.Writer
	Now func() time.Time
	// DryRun only changes what the flow says. What keeps a dry run safe is
	// that Ops is a DryRunOps, which can't write.
	DryRun bool
	reader *bufio.Reader
}

// ErrStopped means the run stopped safely, waiting on a person.
var ErrStopped = errors.New("stopped")

// Run walks the stages. It works out where to pick up from what QBO and
// Airtable show, so stopping at any point and rerunning is safe.
func (f *Flow) Run(ctx context.Context) error {
	f.reader = bufio.NewReader(f.In)
	if f.Now == nil {
		f.Now = time.Now
	}

	if f.DryRun {
		fmt.Fprintln(f.Out, "DRY RUN. This uses real data and the real prompts, but nothing is created, emailed, or changed.")
		fmt.Fprintln(f.Out, "Practice approvals don't count toward a real run.")
	}
	f.heading("1. Pre-flight checks")
	snap, err := f.Ops.Gather(ctx)
	if err != nil {
		return err
	}
	if snap.Env != "production" {
		fmt.Fprintf(f.Out, "This is the %s environment, not production.\n", snap.Env)
	}
	fmt.Fprintf(f.Out, "Connected to %s (%s).\n", snap.Company, snap.Env)

	findings := Check(f.Cfg, snap)
	summary := Summarize(snap.Plan)
	fmt.Fprintf(f.Out, "Plan for %s, dated %s: %d invoices ($%.2f), %d $0 receipts, %d skipped, %d errors.\n",
		snap.FY.Label, snap.AsOf.Format("2006-01-02"), summary.Invoices, summary.Billed, summary.Receipts, summary.Skipped, summary.Errors)
	fmt.Fprintf(f.Out, "Found %d blockers and %d warnings.\n", CountByLevel(findings, Blocker), CountByLevel(findings, Warning))

	if n := CountByLevel(findings, Blocker); n > 0 {
		PrintFindings(f.Out, findings, Blocker)
		f.writeReview(snap, findings)
		fmt.Fprintf(f.Out, "\nFix the blockers above, then run this again. Nothing was changed.\n")
		return ErrStopped
	}
	if summary.Errors > 0 {
		for _, mp := range snap.Plan.MemberPlans {
			if mp.Action == planner.ActionError {
				fmt.Fprintf(f.Out, "  %s %s: %s\n", mp.Member.ControlNumber, mp.Member.Name, mp.SkipDetail)
			}
		}
		fmt.Fprintln(f.Out, "Fix the errors above, then run this again. Nothing was changed.")
		return ErrStopped
	}

	if summary.Invoices+summary.Receipts > 0 {
		if err := f.reviewAndCreate(ctx, snap, findings, summary); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(f.Out, "Nothing to create.")
	}
	if err := f.send(ctx, snap); err != nil {
		return err
	}
	if f.DryRun {
		fmt.Fprintln(f.Out, "\nDry run finished. Nothing was created, emailed, or changed.")
	}
	return nil
}

func (f *Flow) reviewAndCreate(ctx context.Context, snap *Snapshot, findings []Finding, summary Summary) error {
	f.heading("2. Review")
	review, err := LoadReview(f.Dir)
	if err != nil {
		return err
	}
	fingerprint := Fingerprint(snap.Plan)
	if review != nil && review.Approved && review.Fingerprint == fingerprint {
		fmt.Fprintf(f.Out, "This exact plan was reviewed on %s.\n", review.ApprovedAt)
	} else {
		if review != nil && review.Approved {
			fmt.Fprintln(f.Out, "The plan changed since it was last reviewed:")
			f.printList(DiffPlan(review, snap.Plan))
		}
		path := f.writeReview(snap, findings)
		fmt.Fprintf(f.Out, "Open %s and go through it.\n", path)
		fmt.Fprintf(f.Out, "Flagged members are at the top, with the reason for each flag. Fix anything wrong in Airtable or QBO.\n")
		fmt.Fprintf(f.Out, "If you changed anything, stop here and run this again so the file is rebuilt.\n")
		n := CountByLevel(findings, Warning)
		if !f.confirm(fmt.Sprintf("reviewed %d", n), "to confirm you've handled all the warnings") {
			return ErrStopped
		}
		review = NewReviewState(snap.Plan)
		review.Approved = true
		review.ApprovedAt = stamp()
		if err := SaveReview(f.Dir, review); err != nil {
			return err
		}
	}

	f.heading("3. Create")
	fmt.Fprintf(f.Out, "This creates %d invoices ($%.2f) and %d $0 receipts in QBO, and updates statuses and credits in Airtable.\nNo one is emailed.\n", summary.Invoices, summary.Billed, summary.Receipts)
	if !f.confirm(fmt.Sprintf("create %d + %d", summary.Invoices, summary.Receipts), f.practice("to create them")) {
		return ErrStopped
	}

	// Look again right before writing: this is what stops a late Airtable edit
	// from going out unreviewed.
	fresh, err := f.Ops.Gather(ctx)
	if err != nil {
		return err
	}
	if Fingerprint(fresh.Plan) != review.Fingerprint {
		fmt.Fprintln(f.Out, "The plan changed since you reviewed it, so nothing was created:")
		f.printList(DiffPlan(review, fresh.Plan))
		fmt.Fprintln(f.Out, "Run this again to review the new plan.")
		return ErrStopped
	}

	created, err := f.Ops.Create(ctx, fresh.Plan)
	if err != nil {
		return err
	}

	f.heading("4. Verify create")
	if problems := f.Ops.VerifyCreated(ctx, created); len(problems) > 0 {
		fmt.Fprintf(f.Out, "%d problems found. Nothing has been emailed:\n", len(problems))
		f.printList(problems)
		fmt.Fprintln(f.Out, "Fix these in QBO or Airtable, then run this again. If a status update failed, set that member's status by hand first, or a rerun can bill them twice.")
		return ErrStopped
	}
	if f.DryRun {
		fmt.Fprintln(f.Out, "Simulated. A real run reads every document back from QBO here and compares it with the reviewed plan.")
		return nil
	}
	fmt.Fprintln(f.Out, "Every document in QBO matches the reviewed plan.")
	return nil
}

func (f *Flow) send(ctx context.Context, snap *Snapshot) error {
	plan, err := f.Ops.SendPlan(ctx)
	if err != nil {
		return err
	}
	var invoices, receipts int
	var total float64
	for _, mp := range plan.MemberPlans {
		switch mp.Action {
		case planner.ActionSend:
			invoices++
			total += mp.ExistingInvoice.TotalAmt
		case planner.ActionSendReceipt:
			receipts++
		}
	}
	if invoices+receipts == 0 {
		fmt.Fprintln(f.Out, "\nNothing left to send. Run 'invoicer dues-run status' to follow up.")
		return nil
	}

	f.heading("5. Send")
	fmt.Fprintf(f.Out, "This emails %d invoices ($%.2f) and %d $0 receipts. Emails can't be recalled.\n", invoices, total, receipts)
	fmt.Fprintln(f.Out, "Spot-check a few documents in QBO first. You can stop here and come back: rerunning picks up at this step.")
	if f.Now().Before(snap.AsOf) {
		fmt.Fprintf(f.Out, "Warning: documents are dated %s, which hasn't arrived yet, and QBO emails right away.\n", snap.AsOf.Format("2006-01-02"))
	}
	if !f.confirm(fmt.Sprintf("send %d + %d", invoices, receipts), f.practice("to email them")) {
		return ErrStopped
	}
	sent, err := f.Ops.Send(ctx, plan)
	if err != nil {
		return err
	}

	f.heading("6. Verify send")
	if problems := f.Ops.VerifySent(ctx, sent); len(problems) > 0 {
		fmt.Fprintf(f.Out, "%d problems found:\n", len(problems))
		f.printList(problems)
		fmt.Fprintln(f.Out, "Rerunning send is safe: anything already emailed is skipped.")
		return ErrStopped
	}
	if f.DryRun {
		fmt.Fprintln(f.Out, "Simulated. A real run confirms every document shows as sent here.")
	} else {
		fmt.Fprintln(f.Out, "Everything shows as sent.")
	}
	fmt.Fprintln(f.Out, "\nNext: bounces can take hours to appear, and a bounced invoice still shows as sent.")
	fmt.Fprintln(f.Out, "Run 'invoicer dues-run status' later today and tomorrow.")
	return nil
}

func (f *Flow) writeReview(snap *Snapshot, findings []Finding) string {
	path, err := WriteReviewFile(f.Dir, snap, findings)
	if err != nil {
		fmt.Fprintf(f.Out, "Couldn't write the review file: %v\n", err)
	}
	return path
}

// confirm requires the exact phrase. Anything else, including a bare Enter or
// the end of input, stops the run.
func (f *Flow) confirm(phrase, purpose string) bool {
	fmt.Fprintf(f.Out, "\nType %q %s, or anything else to stop: ", phrase, purpose)
	line, _ := f.reader.ReadString('\n')
	if strings.TrimSpace(line) == phrase {
		return true
	}
	fmt.Fprintln(f.Out, "\nStopped. Rerun to pick up here.")
	return false
}

func (f *Flow) heading(s string) {
	if f.DryRun {
		s += " (dry run)"
	}
	fmt.Fprintf(f.Out, "\n== %s ==\n", s)
}

func (f *Flow) practice(purpose string) string {
	if f.DryRun {
		return purpose + " (practice only)"
	}
	return purpose
}

func (f *Flow) printList(items []string) {
	for _, s := range items {
		fmt.Fprintf(f.Out, "  %s\n", s)
	}
}

// PrintFindings lists findings at a level, or all of them if level is blank.
func PrintFindings(w io.Writer, findings []Finding, level Level) {
	for _, f := range findings {
		if level != "" && f.Level != level {
			continue
		}
		who := ""
		if f.Control != "" {
			who = fmt.Sprintf("%s %s: ", f.Control, f.Name)
		}
		fmt.Fprintf(w, "  [%s] %s%s\n", f.Level, who, f.Message)
	}
}
