package qbo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/storage"
)

type Client struct {
	cfg     *config.Config
	token   *storage.Token
	http    *http.Client
	realmID string
}

func NewClient(cfg *config.Config) (*Client, error) {
	tok, err := storage.LoadToken(cfg)
	if err != nil {
		return nil, fmt.Errorf("loading QBO token: %w\nRun 'invoicer auth' to authenticate", err)
	}
	return &Client{
		cfg:     cfg,
		token:   tok,
		http:    &http.Client{Timeout: 30 * time.Second},
		realmID: tok.RealmID,
	}, nil
}

func (c *Client) baseURL() string {
	return c.cfg.QBO.BaseURL()
}

func (c *Client) query(ctx context.Context, q string) ([]byte, error) {
	u := fmt.Sprintf("%s/v3/company/%s/query?query=%s&minorversion=65",
		c.baseURL(), c.realmID, url.QueryEscape(q))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	return c.do(req)
}

func (c *Client) post(ctx context.Context, entity string, payload interface{}) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("%s/v3/company/%s/%s?minorversion=65", c.baseURL(), c.realmID, entity)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	return c.do(req)
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token.AccessToken)
	req.Header.Set("Accept", "application/json")
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("QBO authentication expired; run 'invoicer auth' to re-authenticate")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("QBO %s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, sanitizeBody(body))
	}
	return body, nil
}

// sanitizeBody removes potential token values from error bodies before logging.
func sanitizeBody(b []byte) string {
	s := string(b)
	if len(s) > 500 {
		s = s[:500] + "...[truncated]"
	}
	return s
}

// NextInvoiceNumber queries the 100 most recent invoices, finds the numerically
// largest DocNumber, and returns an incremented version preserving any prefix and zero-padding.
func (c *Client) NextInvoiceNumber(ctx context.Context) (string, error) {
	q := "SELECT * FROM Invoice ORDERBY MetaData.CreateTime DESC MAXRESULTS 100"
	body, err := c.query(ctx, q)
	if err != nil {
		return "", err
	}

	var result struct {
		QueryResponse struct {
			Invoice []map[string]interface{} `json:"Invoice"`
		} `json:"QueryResponse"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	var maxNum int
	var bestDoc string
	for _, raw := range result.QueryResponse.Invoice {
		doc := stringVal(raw, "DocNumber")
		if n := trailingInt(doc); n > maxNum {
			maxNum = n
			bestDoc = doc
		}
	}

	return incrementDocNumber(bestDoc, maxNum), nil
}

// trailingInt extracts the trailing numeric portion of a string as an int.
func trailingInt(s string) int {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return 0
	}
	n, _ := strconv.Atoi(s[i:])
	return n
}

// IncrementDocNumber increments the trailing numeric portion of a DocNumber,
// preserving any prefix and zero-padding.
func IncrementDocNumber(last string) string {
	return incrementDocNumber(last, trailingInt(last))
}

func incrementDocNumber(last string, lastNum int) string {
	if last == "" {
		return "1"
	}
	i := len(last)
	for i > 0 && last[i-1] >= '0' && last[i-1] <= '9' {
		i--
	}
	prefix := last[:i]
	numStr := last[i:]
	if numStr == "" {
		return last + "1"
	}
	next := strconv.Itoa(lastNum + 1)
	if len(numStr) > 1 && numStr[0] == '0' {
		for len(next) < len(numStr) {
			next = "0" + next
		}
	}
	return prefix + next
}

// ListItems returns all active QBO items (products and services).
func (c *Client) ListItems(ctx context.Context) ([]Item, error) {
	q := "SELECT * FROM Item WHERE Active = true MAXRESULTS 1000"
	body, err := c.query(ctx, q)
	if err != nil {
		return nil, err
	}

	var result struct {
		QueryResponse struct {
			Item []map[string]interface{} `json:"Item"`
		} `json:"QueryResponse"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(result.QueryResponse.Item))
	for _, raw := range result.QueryResponse.Item {
		items = append(items, Item{
			ID:          stringVal(raw, "Id"),
			Name:        stringVal(raw, "Name"),
			Description: stringVal(raw, "Description"),
			Type:        stringVal(raw, "Type"),
			Active:      boolVal(raw, "Active"),
		})
	}
	return items, nil
}

// CreateCustomer creates a new QBO customer record.
func (c *Client) CreateCustomer(ctx context.Context, displayName, email, notes string) (*Customer, error) {
	payload := map[string]interface{}{
		"DisplayName": displayName,
		"Notes":       notes,
	}
	if email != "" {
		payload["PrimaryEmailAddr"] = map[string]interface{}{"Address": email}
	}

	body, err := c.post(ctx, "customer", payload)
	if err != nil {
		return nil, err
	}

	var result struct {
		Customer map[string]interface{} `json:"Customer"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	cust := parseCustomer(result.Customer)
	return &cust, nil
}

// ListCustomers returns all active QBO customers.
func (c *Client) ListCustomers(ctx context.Context) ([]Customer, error) {
	q := "SELECT * FROM Customer WHERE Active = true MAXRESULTS 1000"
	body, err := c.query(ctx, q)
	if err != nil {
		return nil, err
	}

	var result struct {
		QueryResponse struct {
			Customer []map[string]interface{} `json:"Customer"`
		} `json:"QueryResponse"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	customers := make([]Customer, 0, len(result.QueryResponse.Customer))
	for _, raw := range result.QueryResponse.Customer {
		customers = append(customers, parseCustomer(raw))
	}
	return customers, nil
}

// GetCustomer fetches a single customer by ID.
func (c *Client) GetCustomer(ctx context.Context, customerID string) (*Customer, error) {
	q := fmt.Sprintf("SELECT * FROM Customer WHERE Id = '%s'", customerID)
	body, err := c.query(ctx, q)
	if err != nil {
		return nil, err
	}

	var result struct {
		QueryResponse struct {
			Customer []map[string]interface{} `json:"Customer"`
		} `json:"QueryResponse"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if len(result.QueryResponse.Customer) == 0 {
		return nil, fmt.Errorf("QBO customer %q not found", customerID)
	}
	c2 := parseCustomer(result.QueryResponse.Customer[0])
	return &c2, nil
}

// QueryInvoices returns invoices for a customer within a date range.
func (c *Client) QueryInvoices(ctx context.Context, customerID string, periodStart, periodEnd time.Time) ([]Invoice, error) {
	q := fmt.Sprintf(
		"SELECT * FROM Invoice WHERE CustomerRef = '%s' AND TxnDate >= '%s' AND TxnDate <= '%s'",
		customerID,
		periodStart.Format("2006-01-02"),
		periodEnd.Format("2006-01-02"),
	)
	body, err := c.query(ctx, q)
	if err != nil {
		return nil, err
	}

	var result struct {
		QueryResponse struct {
			Invoice []map[string]interface{} `json:"Invoice"`
		} `json:"QueryResponse"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	invoices := make([]Invoice, 0, len(result.QueryResponse.Invoice))
	for _, raw := range result.QueryResponse.Invoice {
		invoices = append(invoices, parseInvoice(raw))
	}
	return invoices, nil
}

// CreateInvoice creates a new invoice in QBO.
func (c *Client) CreateInvoice(ctx context.Context, req InvoiceCreateRequest) (*Invoice, error) {
	payload := buildInvoicePayload(req)
	body, err := c.post(ctx, "invoice", payload)
	if err != nil {
		return nil, err
	}

	var result struct {
		Invoice map[string]interface{} `json:"Invoice"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	inv := parseInvoice(result.Invoice)
	return &inv, nil
}

// SendInvoice sends an invoice via QBO email.
func (c *Client) SendInvoice(ctx context.Context, invoiceID string) error {
	u := fmt.Sprintf("%s/v3/company/%s/invoice/%s/send?minorversion=65",
		c.baseURL(), c.realmID, invoiceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(""))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/octet-stream")

	_, err = c.do(req)
	return err
}

// GetInvoice fetches a single invoice by ID.
func (c *Client) GetInvoice(ctx context.Context, invoiceID string) (*Invoice, error) {
	u := fmt.Sprintf("%s/v3/company/%s/invoice/%s?minorversion=65",
		c.baseURL(), c.realmID, invoiceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	body, err := c.do(req)
	if err != nil {
		return nil, err
	}

	var result struct {
		Invoice map[string]interface{} `json:"Invoice"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	inv := parseInvoice(result.Invoice)
	return &inv, nil
}

func parseCustomer(raw map[string]interface{}) Customer {
	c := Customer{
		ID:     stringVal(raw, "Id"),
		Active: boolVal(raw, "Active"),
	}
	if dn, ok := raw["DisplayName"].(string); ok {
		c.DisplayName = dn
	}
	if notes, ok := raw["Notes"].(string); ok {
		c.Notes = notes
	}
	if email, ok := raw["PrimaryEmailAddr"].(map[string]interface{}); ok {
		c.Email = stringVal(email, "Address")
	}
	return c
}

func parseInvoice(raw map[string]interface{}) Invoice {
	inv := Invoice{
		ID:          stringVal(raw, "Id"),
		DocNumber:   stringVal(raw, "DocNumber"),
		TotalAmt:    floatVal(raw, "TotalAmt"),
		Balance:     floatVal(raw, "Balance"),
		EmailStatus: stringVal(raw, "EmailStatus"),
		PrivateNote: stringVal(raw, "PrivateNote"),
		SyncToken:   stringVal(raw, "SyncToken"),
	}
	if cr, ok := raw["CustomerRef"].(map[string]interface{}); ok {
		inv.CustomerRef = CustomerRef{
			Value: stringVal(cr, "value"),
			Name:  stringVal(cr, "name"),
		}
	}
	if txn := stringVal(raw, "TxnDate"); txn != "" {
		inv.TxnDate, _ = time.Parse("2006-01-02", txn)
	}
	if due := stringVal(raw, "DueDate"); due != "" {
		inv.DueDate, _ = time.Parse("2006-01-02", due)
	}
	return inv
}

func buildInvoicePayload(req InvoiceCreateRequest) map[string]interface{} {
	lines := make([]map[string]interface{}, 0, len(req.Line))
	for _, l := range req.Line {
		line := map[string]interface{}{
			"Amount":     l.Amount,
			"DetailType": "SalesItemLineDetail",
		}
		if l.SalesItemLineDetail != nil {
			line["SalesItemLineDetail"] = map[string]interface{}{
				"ItemRef": map[string]interface{}{
					"value": l.SalesItemLineDetail.ItemRef.Value,
				},
				"Qty":       l.SalesItemLineDetail.Qty,
				"UnitPrice": l.SalesItemLineDetail.UnitPrice,
			}
		}
		if l.Description != "" {
			line["Description"] = l.Description
		}
		lines = append(lines, line)
	}

	payload := map[string]interface{}{
		"CustomerRef": map[string]interface{}{
			"value": req.CustomerRef.Value,
		},
		"TxnDate":     req.TxnDate,
		"DueDate":     req.DueDate,
		"PrivateNote": req.PrivateNote,
		"Line":        lines,
	}
	if req.DocNumber != "" {
		payload["DocNumber"] = req.DocNumber
	}
	if req.CustomerMemo != "" {
		payload["CustomerMemo"] = map[string]interface{}{"value": req.CustomerMemo}
	}
	if req.BillEmail != "" {
		payload["BillEmail"] = map[string]interface{}{"Address": req.BillEmail}
	}
	if req.SalesTermID != "" {
		payload["SalesTermRef"] = map[string]interface{}{"value": req.SalesTermID}
	}
	return payload
}

func stringVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func floatVal(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

func boolVal(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}
