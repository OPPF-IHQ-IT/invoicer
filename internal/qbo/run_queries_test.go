package qbo

import "testing"

func TestParseInvoiceReadsLinesAndBounces(t *testing.T) {
	inv := parseInvoice(map[string]any{
		"Id": "1", "TotalAmt": 408.0,
		"BillEmail":    map[string]any{"Address": "a@b.c"},
		"DeliveryInfo": map[string]any{"DeliveryType": "Email", "DeliveryErrorType": "Undeliverable"},
		"Line": []any{
			map[string]any{"DetailType": "SalesItemLineDetail", "Amount": 408.0, "Description": "Dues", "SalesItemLineDetail": map[string]any{
				"ItemRef": map[string]any{"value": "79", "name": "Local"}, "Qty": 1.0, "UnitPrice": 408.0,
			}},
			map[string]any{"DetailType": "SubTotalLineDetail", "Amount": 408.0},
		},
	})
	if inv.BillEmail != "a@b.c" || inv.DeliveryError != "Undeliverable" {
		t.Fatalf("got %+v", inv)
	}
	if len(inv.Line) != 1 || inv.Line[0].SalesItemLineDetail.ItemRef.Value != "79" {
		t.Fatalf("lines %+v", inv.Line)
	}
}
