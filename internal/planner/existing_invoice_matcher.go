package planner

import (
	"strings"

	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

const invoicerMarkerPrefix = "invoicer "

// matchExistingInvoice finds invoices that represent the same billing obligation.
// It first checks for invoicer metadata markers, then falls back to line-item hash comparison.
func matchExistingInvoice(invoices []qbo.Invoice, lineHash string, fy fiscal.Year) []*qbo.Invoice {
	// Pass 1: invoices with matching invoicer metadata marker.
	var markerMatches []*qbo.Invoice
	for i := range invoices {
		if invoiceMatchesMarker(&invoices[i], fy, lineHash) {
			markerMatches = append(markerMatches, &invoices[i])
		}
	}
	if len(markerMatches) > 0 {
		return markerMatches
	}

	// Pass 2: line-item hash match for invoices without marker.
	var hashMatches []*qbo.Invoice
	for i := range invoices {
		if invoiceMatchesHash(&invoices[i], lineHash) {
			hashMatches = append(hashMatches, &invoices[i])
		}
	}
	return hashMatches
}

func invoiceMatchesMarker(inv *qbo.Invoice, fy fiscal.Year, lineHash string) bool {
	if !strings.HasPrefix(inv.PrivateNote, invoicerMarkerPrefix) {
		return false
	}
	return strings.Contains(inv.PrivateNote, "fiscal_year="+fy.Label) &&
		strings.Contains(inv.PrivateNote, "line_hash="+lineHash)
}

func invoiceMatchesHash(inv *qbo.Invoice, lineHash string) bool {
	return strings.Contains(inv.PrivateNote, "line_hash="+lineHash)
}

// BuildPrivateNote constructs the invoicer metadata string for a new invoice.
func BuildPrivateNote(fy fiscal.Year, airtableRecordID, lineHash, runID string) string {
	return strings.Join([]string{
		invoicerMarkerPrefix,
		"fiscal_year=" + fy.Label,
		"period=" + fy.PeriodStart.Format("2006-01-02") + ".." + fy.PeriodEnd.Format("2006-01-02"),
		"airtable_record_id=" + airtableRecordID,
		"line_hash=" + lineHash,
		"run_id=" + runID,
	}, " ")
}
