package airtable

import (
	"context"
	"net/url"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
)

// ListInvoiceableMembers returns members whose Status is in the configured invoiceable list.
func (c *Client) ListInvoiceableMembers(ctx context.Context, cfg *config.AirtableConfig) ([]Member, error) {
	f := cfg.Fields.Members

	filterParts := make([]string, 0, len(cfg.InvoiceableStatuses))
	for _, s := range cfg.InvoiceableStatuses {
		filterParts = append(filterParts, "{"+f.Status+"}=\""+s+"\"")
	}

	formula := filterParts[0]
	if len(filterParts) > 1 {
		joined := ""
		for _, p := range filterParts {
			joined += p + ","
		}
		formula = "OR(" + joined[:len(joined)-1] + ")"
	}

	params := url.Values{}
	params.Set("filterByFormula", formula)
	if cfg.Views.Members != "" {
		params.Set("view", cfg.Views.Members)
	}

	records, err := c.listRecords(ctx, cfg.Tables.Members, params)
	if err != nil {
		return nil, err
	}

	members := make([]Member, 0, len(records))
	for _, r := range records {
		members = append(members, Member{
			RecordID:         r.ID,
			ControlNumber:    stringField(r, f.ControlNumber),
			Email:            stringField(r, f.Email),
			Status:           stringField(r, f.Status),
			QBOCustomerID:    stringField(r, f.QBOCustomerID),
			IntlLife:         boolField(r, f.IntlLife),
			DistrictLife:     boolField(r, f.DistrictLife),
			StateLife:        boolField(r, f.StateLife),
			LocalLife:        boolField(r, f.LocalLife),
			BasileusEmeritus: boolField(r, f.BasileusEmeritus),
			Retired:          boolField(r, f.Retired),
		})
	}
	return members, nil
}

// UpdateMemberStatus sets the Status field on a member record.
func (c *Client) UpdateMemberStatus(ctx context.Context, cfg *config.AirtableConfig, recordID, status string) error {
	return c.patchRecord(ctx, cfg.Tables.Members, recordID, map[string]interface{}{
		cfg.Fields.Members.Status: status,
	})
}

// UpdateMemberQBOCustomerID sets the QBO Customer ID field on a member record.
func (c *Client) UpdateMemberQBOCustomerID(ctx context.Context, cfg *config.AirtableConfig, recordID, customerID string) error {
	return c.patchRecord(ctx, cfg.Tables.Members, recordID, map[string]interface{}{
		cfg.Fields.Members.QBOCustomerID: customerID,
	})
}

// ListAllMembers returns all members regardless of status (used for reconciliation).
func (c *Client) ListAllMembers(ctx context.Context, cfg *config.AirtableConfig) ([]Member, error) {
	f := cfg.Fields.Members
	params := url.Values{}
	if cfg.Views.Members != "" {
		params.Set("view", cfg.Views.Members)
	}

	records, err := c.listRecords(ctx, cfg.Tables.Members, params)
	if err != nil {
		return nil, err
	}

	members := make([]Member, 0, len(records))
	for _, r := range records {
		members = append(members, Member{
			RecordID:         r.ID,
			ControlNumber:    stringField(r, f.ControlNumber),
			Email:            stringField(r, f.Email),
			Status:           stringField(r, f.Status),
			QBOCustomerID:    stringField(r, f.QBOCustomerID),
			IntlLife:         boolField(r, f.IntlLife),
			DistrictLife:     boolField(r, f.DistrictLife),
			StateLife:        boolField(r, f.StateLife),
			LocalLife:        boolField(r, f.LocalLife),
			BasileusEmeritus: boolField(r, f.BasileusEmeritus),
			Retired:          boolField(r, f.Retired),
		})
	}
	return members, nil
}
