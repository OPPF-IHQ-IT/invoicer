package airtable

import (
	"context"
	"net/url"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
)

// CreditRow is one credit row with every member it links to, used to spot rows
// the calculator can't apply cleanly.
type CreditRow struct {
	RecordID  string
	MemberIDs []string
	Earned    float64
	Spent     float64
	Available float64
}

// ListPollWorkerCreditRows returns every poll worker credit row.
func (c *Client) ListPollWorkerCreditRows(ctx context.Context, cfg *config.AirtableConfig) ([]CreditRow, error) {
	f := cfg.Fields.PollWorkerCredits
	records, err := c.listRecords(ctx, cfg.Tables.PollWorkerCreditUtilization, url.Values{})
	if err != nil {
		return nil, err
	}
	rows := make([]CreditRow, 0, len(records))
	for _, r := range records {
		rows = append(rows, CreditRow{
			RecordID:  r.ID,
			MemberIDs: linkedRecordIDs(r, f.MemberLink),
			Earned:    floatField(r, f.CreditsEarned),
			Spent:     floatField(r, f.CreditsSpent),
			Available: floatField(r, f.CreditsAvailable),
		})
	}
	return rows, nil
}

// ListMemberCreditRows returns every Member Credits row, including ones the
// calculator ignores. It returns nil when no table is configured.
func (c *Client) ListMemberCreditRows(ctx context.Context, cfg *config.AirtableConfig) ([]CreditRow, error) {
	if cfg.Tables.MemberCredits == "" {
		return nil, nil
	}
	f := cfg.Fields.MemberCredits
	records, err := c.listRecords(ctx, cfg.Tables.MemberCredits, url.Values{})
	if err != nil {
		return nil, err
	}
	rows := make([]CreditRow, 0, len(records))
	for _, r := range records {
		earned, spent := floatField(r, f.Earned), floatField(r, f.Spent)
		rows = append(rows, CreditRow{
			RecordID:  r.ID,
			MemberIDs: linkedRecordIDs(r, f.MemberLink),
			Earned:    earned,
			Spent:     spent,
			Available: earned - spent,
		})
	}
	return rows, nil
}
