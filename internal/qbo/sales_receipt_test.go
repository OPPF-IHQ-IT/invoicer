package qbo

import "testing"

func TestBuildSalesReceiptPayload(t *testing.T) {
	line := InvoiceLine{Amount: -148, Description: "Offset", SalesItemLineDetail: &SalesItemLineDetail{
		ItemRef: ItemRef{Value: "14"}, Qty: 1, UnitPrice: -148,
	}}
	p := buildSalesReceiptPayload(SalesReceiptCreateRequest{
		CustomerRef: CustomerRef{Value: "182"}, DocNumber: "1681", TxnDate: "2027-11-01",
		PrivateNote: "note", CustomerMemo: "Thanks", BillEmail: "a@b.c",
		DepositAccountID: "115", Line: []InvoiceLine{line},
	})
	if _, ok := p["DueDate"]; ok {
		t.Error("a sales receipt must not have a DueDate")
	}
	if got := p["DepositToAccountRef"].(map[string]any)["value"]; got != "115" {
		t.Errorf("DepositToAccountRef = %v", got)
	}
	if got := len(p["Line"].([]map[string]any)); got != 1 {
		t.Errorf("lines = %d", got)
	}
	if got := p["DocNumber"]; got != "1681" {
		t.Errorf("DocNumber = %v", got)
	}

	noDeposit := buildSalesReceiptPayload(SalesReceiptCreateRequest{CustomerRef: CustomerRef{Value: "1"}})
	if _, ok := noDeposit["DepositToAccountRef"]; ok {
		t.Error("blank deposit account should leave the choice to QBO")
	}
}

func TestWithClassStampsLinePayload(t *testing.T) {
	lines := []InvoiceLine{
		{Amount: 248, SalesItemLineDetail: &SalesItemLineDetail{ItemRef: ItemRef{Value: "13"}, Qty: 1, UnitPrice: 248}},
		{Amount: -50, SalesItemLineDetail: &SalesItemLineDetail{ItemRef: ItemRef{Value: "97"}, Qty: 1, UnitPrice: -50, ClassID: "999"}},
	}
	classed := WithClass(lines, "1543455")
	if lines[0].SalesItemLineDetail.ClassID != "" {
		t.Fatal("WithClass must not mutate the caller's lines")
	}
	p := buildInvoicePayload(InvoiceCreateRequest{CustomerRef: CustomerRef{Value: "1"}, Line: classed})
	got := p["Line"].([]map[string]any)
	if v := got[0]["SalesItemLineDetail"].(map[string]any)["ClassRef"].(map[string]any)["value"]; v != "1543455" {
		t.Errorf("line 0 ClassRef = %v, want 1543455", v)
	}
	if v := got[1]["SalesItemLineDetail"].(map[string]any)["ClassRef"].(map[string]any)["value"]; v != "999" {
		t.Errorf("line 1 ClassRef = %v, want existing 999 kept", v)
	}

	plain := buildInvoicePayload(InvoiceCreateRequest{CustomerRef: CustomerRef{Value: "1"}, Line: WithClass(lines[:1], "")})
	if _, ok := plain["Line"].([]map[string]any)[0]["SalesItemLineDetail"].(map[string]any)["ClassRef"]; ok {
		t.Error("blank class must send no ClassRef")
	}
}

func TestParseSalesReceipt(t *testing.T) {
	sr := parseSalesReceipt(map[string]any{
		"Id": "4490", "DocNumber": "1681", "TotalAmt": 0.0, "EmailStatus": "EmailSent",
		"PrivateNote": "n", "TxnDate": "2026-11-01",
		"CustomerRef": map[string]any{"value": "182", "name": "S"},
	})
	if sr.ID != "4490" || sr.EmailStatus != "EmailSent" || sr.CustomerRef.Value != "182" || sr.TxnDate.Month() != 11 {
		t.Errorf("parsed %+v", sr)
	}
}
