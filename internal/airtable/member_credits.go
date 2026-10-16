package airtable

import (
	"context"
	"net/url"
	"sort"
	"strings"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
)

// ListMemberCredits returns every Member Credits row with credit remaining,
// keyed by linked member record ID and ordered oldest first. It returns nil
// when no Member Credits table is configured.
func (c *Client) ListMemberCredits(ctx context.Context, cfg *config.AirtableConfig) (map[string][]MemberCredit, error) {
	if cfg.Tables.MemberCredits == "" {
		return nil, nil
	}
	records, err := c.listRecords(ctx, cfg.Tables.MemberCredits, url.Values{})
	if err != nil {
		return nil, err
	}
	return parseMemberCredits(records, cfg.Fields.MemberCredits), nil
}

func parseMemberCredits(records []record, f config.MemberCreditsFieldsConfig) map[string][]MemberCredit {
	byMember := make(map[string][]MemberCredit)
	for _, r := range records {
		credit := MemberCredit{
			RecordID:    r.ID,
			Type:        stringField(r, f.Type),
			LocalOnly:   strings.EqualFold(strings.TrimSpace(stringField(r, f.AppliesTo)), "Local Only"),
			Earned:      floatField(r, f.Earned),
			Spent:       floatField(r, f.Spent),
			MaxPerYear:  floatField(r, f.MaxPerYear),
			CreatedTime: r.CreatedTime,
		}
		if credit.Available() <= 0 {
			continue
		}
		// A credit belongs to one brother. If a row links several, only the
		// first is used so the same dollars are never applied twice.
		ids := linkedRecordIDs(r, f.MemberLink)
		if len(ids) == 0 {
			continue
		}
		credit.MemberID = ids[0]
		byMember[ids[0]] = append(byMember[ids[0]], credit)
	}
	for _, credits := range byMember {
		sort.SliceStable(credits, func(i, j int) bool {
			return credits[i].CreatedTime < credits[j].CreatedTime
		})
	}
	return byMember
}

// SetMemberCreditSpent records the new Spent total on a Member Credits row.
func (c *Client) SetMemberCreditSpent(ctx context.Context, cfg *config.AirtableConfig, recordID string, spent float64) error {
	return c.patchRecord(ctx, cfg.Tables.MemberCredits, recordID, map[string]any{
		cfg.Fields.MemberCredits.Spent: spent,
	})
}
