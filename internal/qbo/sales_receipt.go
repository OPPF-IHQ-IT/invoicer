package qbo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SalesReceipt is the confirmation sent to members who owe nothing for the
// fiscal year. It carries the same lines a $0 invoice would, but reads as
// "you're all set" rather than as a bill.
type SalesReceipt struct {
	ID          string
	DocNumber   string
	CustomerRef CustomerRef
	TxnDate     time.Time
	TotalAmt    float64
	EmailStatus string // NotSet, NeedToSend, EmailSent
	PrivateNote string
	BillEmail   string
	DeliveryError string
	Line        []InvoiceLine
}

// SalesReceiptCreateRequest holds the fields for a new sales receipt.
type SalesReceiptCreateRequest struct {
	CustomerRef      CustomerRef
	DocNumber        string
	TxnDate          string
	PrivateNote      string
	CustomerMemo     string
	BillEmail        string
	DepositAccountID string // blank lets QBO use its default (Undeposited Funds)
	Line             []InvoiceLine
}

// CreateSalesReceipt creates a sales receipt in QBO. It does not email it.
//
// The API drops BillEmailCc/BillEmailBcc on sales receipts, so they aren't
// sent. QBO applies the company's Sales form copies when the receipt is sent.
func (c *Client) CreateSalesReceipt(ctx context.Context, req SalesReceiptCreateRequest) (*SalesReceipt, error) {
	body, err := c.post(ctx, "salesreceipt", buildSalesReceiptPayload(req))
	if err != nil {
		return nil, err
	}

	var result struct {
		SalesReceipt map[string]any `json:"SalesReceipt"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	sr := parseSalesReceipt(result.SalesReceipt)
	return &sr, nil
}

// QuerySalesReceipts returns sales receipts for a customer within a date range.
func (c *Client) QuerySalesReceipts(ctx context.Context, customerID string, periodStart, periodEnd time.Time) ([]SalesReceipt, error) {
	return c.querySalesReceipts(ctx, fmt.Sprintf(
		"SELECT * FROM SalesReceipt WHERE CustomerRef = '%s' AND TxnDate >= '%s' AND TxnDate <= '%s'",
		customerID,
		periodStart.Format("2006-01-02"),
		periodEnd.Format("2006-01-02"),
	))
}

// GetSalesReceipt fetches a single sales receipt by ID.
func (c *Client) GetSalesReceipt(ctx context.Context, id string) (*SalesReceipt, error) {
	receipts, err := c.querySalesReceipts(ctx, fmt.Sprintf("SELECT * FROM SalesReceipt WHERE Id = '%s'", id))
	if err != nil {
		return nil, err
	}
	if len(receipts) == 0 {
		return nil, fmt.Errorf("sales receipt %s not found", id)
	}
	return &receipts[0], nil
}

// SendSalesReceipt emails a sales receipt via QBO.
func (c *Client) SendSalesReceipt(ctx context.Context, id string) error {
	u := fmt.Sprintf("%s/v3/company/%s/salesreceipt/%s/send?minorversion=65", c.baseURL(), c.realmID, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(""))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/octet-stream")

	_, err = c.do(req)
	return err
}

func (c *Client) querySalesReceipts(ctx context.Context, q string) ([]SalesReceipt, error) {
	body, err := c.query(ctx, q)
	if err != nil {
		return nil, err
	}

	var result struct {
		QueryResponse struct {
			SalesReceipt []map[string]any `json:"SalesReceipt"`
		} `json:"QueryResponse"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	receipts := make([]SalesReceipt, 0, len(result.QueryResponse.SalesReceipt))
	for _, raw := range result.QueryResponse.SalesReceipt {
		receipts = append(receipts, parseSalesReceipt(raw))
	}
	return receipts, nil
}

func parseSalesReceipt(raw map[string]any) SalesReceipt {
	sr := SalesReceipt{
		ID:          stringVal(raw, "Id"),
		DocNumber:   stringVal(raw, "DocNumber"),
		TotalAmt:    floatVal(raw, "TotalAmt"),
		EmailStatus: stringVal(raw, "EmailStatus"),
		PrivateNote: stringVal(raw, "PrivateNote"),
		BillEmail:   addressVal(raw, "BillEmail"),
		DeliveryError: deliveryError(raw),
		Line:        parseLines(raw),
	}
	if cr, ok := raw["CustomerRef"].(map[string]any); ok {
		sr.CustomerRef = CustomerRef{Value: stringVal(cr, "value"), Name: stringVal(cr, "name")}
	}
	if txn := stringVal(raw, "TxnDate"); txn != "" {
		sr.TxnDate, _ = time.Parse("2006-01-02", txn)
	}
	return sr
}

func buildSalesReceiptPayload(req SalesReceiptCreateRequest) map[string]any {
	payload := buildInvoicePayload(InvoiceCreateRequest{
		CustomerRef:  req.CustomerRef,
		DocNumber:    req.DocNumber,
		TxnDate:      req.TxnDate,
		PrivateNote:  req.PrivateNote,
		CustomerMemo: req.CustomerMemo,
		BillEmail:    req.BillEmail,
		Line:         req.Line,
	})
	delete(payload, "DueDate") // sales receipts have no due date
	if req.DepositAccountID != "" {
		payload["DepositToAccountRef"] = map[string]any{"value": req.DepositAccountID}
	}
	return payload
}
