package airtable

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://api.airtable.com/v0"

type Client struct {
	apiKey  string
	baseID  string
	http    *http.Client
}

func NewClient(apiKey, baseID string) *Client {
	return &Client{
		apiKey: apiKey,
		baseID: baseID,
		http:   &http.Client{Timeout: 30 * time.Second},
	}
}

type listResponse struct {
	Records []record `json:"records"`
	Offset  string   `json:"offset"`
}

type record struct {
	ID     string                 `json:"id"`
	Fields map[string]any `json:"fields"`
}

// listRecords fetches all records from a table, handling pagination.
func (c *Client) listRecords(ctx context.Context, table string, params url.Values) ([]record, error) {
	var all []record
	for {
		u := fmt.Sprintf("%s/%s/%s?%s", baseURL, c.baseID, url.PathEscape(table), params.Encode())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("airtable GET %s: %w", table, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("airtable GET %s: status %d: %s", table, resp.StatusCode, body)
		}

		var page listResponse
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("airtable decode %s: %w", table, err)
		}
		all = append(all, page.Records...)

		if page.Offset == "" {
			break
		}
		params.Set("offset", page.Offset)
	}
	return all, nil
}

// patchRecord updates specific fields on a single record.
func (c *Client) patchRecord(ctx context.Context, table, recordID string, fields map[string]any) error {
	payload, err := json.Marshal(map[string]any{"fields": fields})
	if err != nil {
		return err
	}

	u := fmt.Sprintf("%s/%s/%s/%s", baseURL, c.baseID, url.PathEscape(table), recordID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u, jsonReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("airtable PATCH %s/%s: %w", table, recordID, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("airtable PATCH %s/%s: status %d: %s", table, recordID, resp.StatusCode, body)
	}
	return nil
}

func stringField(r record, key string) string {
	if v, ok := r.Fields[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func boolField(r record, key string) bool {
	if v, ok := r.Fields[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func floatField(r record, key string) float64 {
	if v, ok := r.Fields[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		}
	}
	return 0
}

func intField(r record, key string) int {
	return int(floatField(r, key))
}
