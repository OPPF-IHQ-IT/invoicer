package invoicecalc

import (
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// HashLineItems produces a stable hash of a normalized line-item set.
// Two invoice line-item sets are considered identical if their hashes match.
func HashLineItems(lines []qbo.InvoiceLine) string {
	type entry struct {
		itemID string
		qty    float64
		rate   float64
		amount float64
	}

	entries := make([]entry, 0, len(lines))
	for _, l := range lines {
		e := entry{amount: l.Amount}
		if l.SalesItemLineDetail != nil {
			e.itemID = l.SalesItemLineDetail.ItemRef.Value
			e.qty = l.SalesItemLineDetail.Qty
			e.rate = l.SalesItemLineDetail.UnitPrice
		}
		entries = append(entries, e)
	}

	// Sort for stability regardless of calculation order.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].itemID != entries[j].itemID {
			return entries[i].itemID < entries[j].itemID
		}
		return entries[i].amount < entries[j].amount
	})

	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%s|%.2f|%.2f|%.2f\n", e.itemID, e.qty, e.rate, e.amount)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
