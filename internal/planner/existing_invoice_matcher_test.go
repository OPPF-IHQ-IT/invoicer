package planner

import (
	"testing"

	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

func TestMatchMemberInvoice(t *testing.T) {
	fy, _ := fiscal.Parse("FY2027")
	prior, _ := fiscal.Parse("FY2026")

	invoices := []qbo.Invoice{
		// Created for recKen; the hash no longer matches today's calculation.
		{ID: "1", PrivateNote: BuildPrivateNote(fy, "recKen", "oldhash", "run1")},
		// Same member, prior fiscal year.
		{ID: "2", PrivateNote: BuildPrivateNote(prior, "recKen", "oldhash", "run0")},
		// Another member whose record ID shares a prefix.
		{ID: "3", PrivateNote: BuildPrivateNote(fy, "recKen2", "h", "run1")},
		// Not created by the invoicer.
		{ID: "4", PrivateNote: "airtable_record_id=recKen fiscal_year=FY2027"},
		// Voided invoices have their note replaced.
		{ID: "5", PrivateNote: "Voided"},
	}

	got := matchMemberInvoice(invoices, fy, "recKen")
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("matchMemberInvoice: got %v, want only invoice 1", ids(got))
	}

	if got := matchMemberInvoice(invoices, fy, "recNobody"); len(got) != 0 {
		t.Errorf("matchMemberInvoice for unknown member: got %v, want none", ids(got))
	}
}

func ids(invs []*qbo.Invoice) []string {
	out := make([]string, len(invs))
	for i, inv := range invs {
		out[i] = inv.ID
	}
	return out
}
