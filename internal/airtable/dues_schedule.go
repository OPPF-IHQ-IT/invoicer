package airtable

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/willmadison/invoicer/internal/config"
)

// ListDuesSchedule returns all dues schedule rows for the given fiscal year label
// plus rows where FiscalYear == "ALL".
func (c *Client) ListDuesSchedule(ctx context.Context, cfg *config.AirtableConfig, airtableFY string) ([]DuesScheduleRow, error) {
	f := cfg.Fields.DuesSchedule

	formula := fmt.Sprintf(`OR({%s}="%s",{%s}="ALL")`, f.FiscalYear, airtableFY, f.FiscalYear)

	params := url.Values{}
	params.Set("filterByFormula", formula)
	if cfg.Views.DuesSchedule != "" {
		params.Set("view", cfg.Views.DuesSchedule)
	}

	records, err := c.listRecords(ctx, cfg.Tables.DuesSchedule, params)
	if err != nil {
		return nil, err
	}

	rows := make([]DuesScheduleRow, 0, len(records))
	for _, r := range records {
		row := DuesScheduleRow{
			RecordID:   r.ID,
			ID:         intField(r, f.ID),
			FiscalYear: stringField(r, f.FiscalYear),
			Level:      stringField(r, f.Level),
			Amount:     floatField(r, f.Amount),
			Key:        stringField(r, f.Key),
		}
		if ds := stringField(r, f.DueDate); ds != "" {
			t, err := time.Parse("2006-01-02", ds)
			if err == nil {
				row.DueDate = &t
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
