package qbo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Term is a QBO payment term.
type Term struct {
	ID   string
	Name string
}

// Payment is a QBO customer payment.
type Payment struct {
	ID           string
	CustomerRef  CustomerRef
	TxnDate      time.Time
	TotalAmt     float64
	UnappliedAmt float64
}

const queryPageSize = 500

// queryAll pages through every row of an entity matching where.
func (c *Client) queryAll(ctx context.Context, entity, where string) ([]map[string]any, error) {
	var rows []map[string]any
	for start := 1; ; start += queryPageSize {
		q := fmt.Sprintf("SELECT * FROM %s %s STARTPOSITION %d MAXRESULTS %d", entity, where, start, queryPageSize)
		body, err := c.query(ctx, q)
		if err != nil {
			return nil, err
		}
		var result struct {
			QueryResponse map[string]json.RawMessage `json:"QueryResponse"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		var page []map[string]any
		if raw, ok := result.QueryResponse[entity]; ok {
			if err := json.Unmarshal(raw, &page); err != nil {
				return nil, err
			}
		}
		rows = append(rows, page...)
		if len(page) < queryPageSize {
			return rows, nil
		}
	}
}

func dateRange(start, end time.Time) string {
	return fmt.Sprintf("WHERE TxnDate >= '%s' AND TxnDate <= '%s'", start.Format("2006-01-02"), end.Format("2006-01-02"))
}

// ListInvoicesBetween returns every invoice dated within a range.
func (c *Client) ListInvoicesBetween(ctx context.Context, start, end time.Time) ([]Invoice, error) {
	rows, err := c.queryAll(ctx, "Invoice", dateRange(start, end))
	if err != nil {
		return nil, err
	}
	invoices := make([]Invoice, 0, len(rows))
	for _, raw := range rows {
		invoices = append(invoices, parseInvoice(raw))
	}
	return invoices, nil
}

// ListSalesReceiptsBetween returns every sales receipt dated within a range.
func (c *Client) ListSalesReceiptsBetween(ctx context.Context, start, end time.Time) ([]SalesReceipt, error) {
	rows, err := c.queryAll(ctx, "SalesReceipt", dateRange(start, end))
	if err != nil {
		return nil, err
	}
	receipts := make([]SalesReceipt, 0, len(rows))
	for _, raw := range rows {
		receipts = append(receipts, parseSalesReceipt(raw))
	}
	return receipts, nil
}

// ListPaymentsBetween returns every customer payment dated within a range.
func (c *Client) ListPaymentsBetween(ctx context.Context, start, end time.Time) ([]Payment, error) {
	rows, err := c.queryAll(ctx, "Payment", dateRange(start, end))
	if err != nil {
		return nil, err
	}
	payments := make([]Payment, 0, len(rows))
	for _, raw := range rows {
		p := Payment{ID: stringVal(raw, "Id"), TotalAmt: floatVal(raw, "TotalAmt"), UnappliedAmt: floatVal(raw, "UnappliedAmt")}
		if cr, ok := raw["CustomerRef"].(map[string]any); ok {
			p.CustomerRef = CustomerRef{Value: stringVal(cr, "value"), Name: stringVal(cr, "name")}
		}
		if txn := stringVal(raw, "TxnDate"); txn != "" {
			p.TxnDate, _ = time.Parse("2006-01-02", txn)
		}
		payments = append(payments, p)
	}
	return payments, nil
}

// ListTerms returns the company's payment terms.
func (c *Client) ListTerms(ctx context.Context) ([]Term, error) {
	rows, err := c.queryAll(ctx, "Term", "")
	if err != nil {
		return nil, err
	}
	terms := make([]Term, 0, len(rows))
	for _, raw := range rows {
		terms = append(terms, Term{ID: stringVal(raw, "Id"), Name: stringVal(raw, "Name")})
	}
	return terms, nil
}

// CompanyName returns the name of the connected QBO company. Because it needs a
// working login for this environment, it also proves the token is good here.
func (c *Client) CompanyName(ctx context.Context) (string, error) {
	rows, err := c.queryAll(ctx, "CompanyInfo", "")
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("QBO returned no company info")
	}
	return stringVal(rows[0], "CompanyName"), nil
}

func addressVal(raw map[string]any, key string) string {
	if m, ok := raw[key].(map[string]any); ok {
		return stringVal(m, "Address")
	}
	return ""
}

func deliveryError(raw map[string]any) string {
	if m, ok := raw["DeliveryInfo"].(map[string]any); ok {
		return stringVal(m, "DeliveryErrorType")
	}
	return ""
}

func parseLines(raw map[string]any) []InvoiceLine {
	rows, ok := raw["Line"].([]any)
	if !ok {
		return nil
	}
	var lines []InvoiceLine
	for _, r := range rows {
		m, ok := r.(map[string]any)
		if !ok || stringVal(m, "DetailType") != "SalesItemLineDetail" {
			continue
		}
		line := InvoiceLine{
			Amount:      floatVal(m, "Amount"),
			DetailType:  "SalesItemLineDetail",
			Description: stringVal(m, "Description"),
		}
		if d, ok := m["SalesItemLineDetail"].(map[string]any); ok {
			detail := &SalesItemLineDetail{Qty: floatVal(d, "Qty"), UnitPrice: floatVal(d, "UnitPrice")}
			if ref, ok := d["ItemRef"].(map[string]any); ok {
				detail.ItemRef = ItemRef{Value: stringVal(ref, "value"), Name: stringVal(ref, "name")}
			}
			line.SalesItemLineDetail = detail
		}
		lines = append(lines, line)
	}
	return lines
}
