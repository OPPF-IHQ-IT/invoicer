package planner

import (
	"testing"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

func TestMatchMemberReceipt(t *testing.T) {
	fy, err := fiscal.Parse("FY2027")
	if err != nil {
		t.Fatal(err)
	}
	note := BuildPrivateNote(fy, "recA", "hash", "run")
	receipts := []qbo.SalesReceipt{
		{ID: "mine", PrivateNote: note},
		{ID: "other-member", PrivateNote: BuildPrivateNote(fy, "recB", "hash", "run")},
		{ID: "hand-made", PrivateNote: "FY2027 confirmation recA"},
	}
	got := matchMemberReceipt(receipts, fy, "recA")
	if len(got) != 1 || got[0].ID != "mine" {
		t.Fatalf("got %+v", got)
	}
}

func TestReceiptCreateAction(t *testing.T) {
	one := []*qbo.SalesReceipt{{ID: "1"}}
	two := []*qbo.SalesReceipt{{ID: "1"}, {ID: "2"}}
	for _, tc := range []struct {
		matched []*qbo.SalesReceipt
		want    Action
	}{{nil, ActionCreateReceipt}, {one, ActionNoChange}, {two, ActionSkip}} {
		if got, _, _ := receiptCreateAction(tc.matched); got != tc.want {
			t.Errorf("%d receipts: got %s, want %s", len(tc.matched), got, tc.want)
		}
	}
}

func TestReceiptSendAction(t *testing.T) {
	unsent := []*qbo.SalesReceipt{{ID: "1", EmailStatus: "NotSet"}}
	sent := []*qbo.SalesReceipt{{ID: "1", EmailStatus: "EmailSent"}}
	two := []*qbo.SalesReceipt{{ID: "1"}, {ID: "2"}}
	for _, tc := range []struct {
		name    string
		matched []*qbo.SalesReceipt
		want    Action
	}{{"none", nil, ActionNoChange}, {"unsent", unsent, ActionSendReceipt}, {"sent", sent, ActionNoChange}, {"dupes", two, ActionSkip}} {
		if got, _, _ := receiptSendAction(tc.matched); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestApplyMSPExpiry(t *testing.T) {
	fy, err := fiscal.Parse("FY2028")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		recent  bool
		through string
		wantMSP bool
		expired bool
	}{
		{true, "FY2027", false, true},  // inclusive: FY2027 crew owes in FY2028
		{true, "FY2028", true, false},  // still covered in its last year
		{true, "FY2029", true, false},  //
		{true, "fy2026 ", false, true}, // forgiving about how it's typed
		{true, "", true, false},        // blank: the box decides
		{true, "soon", true, false},    // unreadable: the box decides
		{false, "FY2027", false, false},
	} {
		got := applyMSPExpiry(airtable.Member{RecentMSP: tc.recent, MSPPrepaidThrough: tc.through}, fy)
		if got.RecentMSP != tc.wantMSP || got.MSPExpired != tc.expired {
			t.Errorf("recent=%v through=%q: got msp=%v expired=%v", tc.recent, tc.through, got.RecentMSP, got.MSPExpired)
		}
	}
}
