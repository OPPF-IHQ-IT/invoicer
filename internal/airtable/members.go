package airtable

import (
	"context"
	"net/url"
	"strings"

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
		members = append(members, memberFromRecord(r, cfg))
	}
	return members, nil
}

// UpdateMemberStatus sets the Status field on a member record.
func (c *Client) UpdateMemberStatus(ctx context.Context, cfg *config.AirtableConfig, recordID, status string) error {
	return c.patchRecord(ctx, cfg.Tables.Members, recordID, map[string]any{
		cfg.Fields.Members.Status: status,
	})
}

// UpdateMemberQBOCustomerID sets the QBO Customer ID field on a member record.
func (c *Client) UpdateMemberQBOCustomerID(ctx context.Context, cfg *config.AirtableConfig, recordID, customerID string) error {
	return c.patchRecord(ctx, cfg.Tables.Members, recordID, map[string]any{
		cfg.Fields.Members.QBOCustomerID: customerID,
	})
}

// UpdateMemberFields writes the provided fields onto a member record. The caller
// is responsible for assembling only the fields that should change so empty
// values aren't accidentally blanked.
func (c *Client) UpdateMemberFields(ctx context.Context, cfg *config.AirtableConfig, recordID string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return c.patchRecord(ctx, cfg.Tables.Members, recordID, fields)
}

// ListAllMembers returns all members regardless of status (used for reconciliation).
func (c *Client) ListAllMembers(ctx context.Context, cfg *config.AirtableConfig) ([]Member, error) {
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
		members = append(members, memberFromRecord(r, cfg))
	}
	return members, nil
}

func memberFromRecord(r record, cfg *config.AirtableConfig) Member {
	f := cfg.Fields.Members
	return Member{
		RecordID:         r.ID,
		ControlNumber:    stringField(r, f.ControlNumber),
		Name:             stringField(r, f.Name),
		Email:            stringField(r, f.Email),
		Status:           stringField(r, f.Status),
		QBOCustomerID:    stringField(r, f.QBOCustomerID),
		IntlLife:         boolField(r, f.IntlLife),
		DistrictLife:     boolField(r, f.DistrictLife),
		StateLife:        boolField(r, f.StateLife),
		LocalLife:        boolField(r, f.LocalLife),
		BasileusEmeritus: boolField(r, f.BasileusEmeritus),
		Retired:          boolField(r, f.Retired),
		RecentMSP:        boolField(r, f.RecentMSP),
		InitiationChapter: stringField(r, f.InitiationChapter),
		ChapterInitiate:   isHomeChapter(stringField(r, f.InitiationChapter), cfg.HomeChapter),
		AtLarge:          boolField(r, f.AtLarge),
		UndergradRate:    boolField(r, f.UndergradRate),
		Reclaimable:      stringField(r, f.Status) == cfg.StatusValues.Reclaimable,
		AddressLine1:     stringField(r, f.AddressLine1),
		AddressLine2:     stringField(r, f.AddressLine2),
		City:             stringField(r, f.City),
		State:            stringField(r, f.State),
		Zip:              stringField(r, f.Zip),
	}
}

// isHomeChapter reports whether a member's initiation chapter is the home
// chapter. Matching ignores case and surrounding/internal whitespace so
// "chi tau" and "Chi  Tau" still count; a blank value never matches.
func isHomeChapter(initiationChapter, homeChapter string) bool {
	norm := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
	got := norm(initiationChapter)
	return got != "" && got == norm(homeChapter)
}
