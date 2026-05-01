package airtable

import (
	"context"
	"net/url"
	"sort"

	"github.com/willmadison/invoicer/internal/config"
)

// ListPollWorkerCredits returns all poll worker credit rows for the given member record IDs.
// Rows with zero available credits are excluded.
func (c *Client) ListPollWorkerCredits(ctx context.Context, cfg *config.AirtableConfig, memberRecordID string) ([]PollWorkerCredit, error) {
	f := cfg.Fields.PollWorkerCredits

	// Filter to rows linked to this member with available credits > 0.
	formula := `AND({` + f.CreditsAvailable + `}>0)`

	params := url.Values{}
	params.Set("filterByFormula", formula)

	records, err := c.listRecords(ctx, cfg.Tables.PollWorkerCreditUtilization, params)
	if err != nil {
		return nil, err
	}

	var credits []PollWorkerCredit
	for _, r := range records {
		// Filter by member link — Airtable linked record fields return arrays of record IDs.
		memberIDs := linkedRecordIDs(r, f.MemberLink)
		matched := false
		for _, id := range memberIDs {
			if id == memberRecordID {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		available := floatField(r, f.CreditsAvailable)
		if available <= 0 {
			continue
		}

		credits = append(credits, PollWorkerCredit{
			RecordID:         r.ID,
			MemberID:         memberRecordID,
			FiscalYear:       stringField(r, f.FiscalYear),
			CreditsEarned:    floatField(r, f.CreditsEarned),
			CreditsSpent:     floatField(r, f.CreditsSpent),
			CreditsAvailable: available,
		})
	}

	// FIFO: sort oldest fiscal year first.
	sort.Slice(credits, func(i, j int) bool {
		return credits[i].FiscalYear < credits[j].FiscalYear
	})

	return credits, nil
}

// ConsumePollWorkerCredits reduces CreditsSpent on the given rows by the amounts consumed.
// amounts[i] is the dollar amount consumed from credits[i].
func (c *Client) ConsumePollWorkerCredits(ctx context.Context, cfg *config.AirtableConfig, credits []PollWorkerCredit, amounts []float64) error {
	f := cfg.Fields.PollWorkerCredits
	for i, credit := range credits {
		if amounts[i] <= 0 {
			continue
		}
		newSpent := credit.CreditsSpent + amounts[i]
		if err := c.patchRecord(ctx, cfg.Tables.PollWorkerCreditUtilization, credit.RecordID, map[string]interface{}{
			f.CreditsSpent: newSpent,
		}); err != nil {
			return err
		}
	}
	return nil
}

// linkedRecordIDs extracts record IDs from an Airtable linked record field (array of strings).
func linkedRecordIDs(r record, key string) []string {
	v, ok := r.Fields[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			ids = append(ids, s)
		}
	}
	return ids
}
