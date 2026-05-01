package invoicecalc

import (
	"fmt"
	"time"

	"github.com/willmadison/invoicer/internal/airtable"
	"github.com/willmadison/invoicer/internal/config"
	"github.com/willmadison/invoicer/internal/fiscal"
	"github.com/willmadison/invoicer/internal/qbo"
)

// SkipReason describes why a member's invoice was not calculated.
type SkipReason string

const (
	SkipReasonNone                    SkipReason = ""
	SkipReasonDuesScheduleMissing     SkipReason = "dues_schedule_missing"
	SkipReasonDuesScheduleConflict    SkipReason = "dues_schedule_conflict"
	SkipReasonQBOItemMappingMissing   SkipReason = "qbo_item_mapping_missing"
	SkipReasonCalculatedTotalZeroOrNeg SkipReason = "calculated_total_zero_or_negative"
	SkipReasonMemberExempt            SkipReason = "member_exempt"
)

// CalculationInput bundles all inputs needed to compute a single member's invoice.
type CalculationInput struct {
	FY              fiscal.Year
	AsOf            time.Time
	Member          airtable.Member
	PollWorkerCredit float64 // total available credits in dollars
	Schedule        []airtable.DuesScheduleRow
	Items           config.QBOItemsConfig
}

// CalculationResult is the output of the calculator for one member.
type CalculationResult struct {
	Lines       []qbo.InvoiceLine
	Total       float64
	LineHash    string
	SkipReason  SkipReason
	SkipDetail  string
	IsZeroTotal bool
}

// Calculate computes the expected invoice line items for a member.
func Calculate(input CalculationInput) CalculationResult {
	sm, skipReason, skipDetail := buildScheduleMap(input.Schedule, input.FY.AirtableLabel)
	if skipReason != SkipReasonNone {
		return CalculationResult{SkipReason: skipReason, SkipDetail: skipDetail}
	}

	lines, skip := buildLineItems(input, sm, input.Items)
	if skip != SkipReasonNone {
		return CalculationResult{SkipReason: skip}
	}

	var total float64
	qboLines := make([]qbo.InvoiceLine, 0, len(lines))
	for _, l := range lines {
		total += l.Amount
		qboLines = append(qboLines, l.toQBOLine())
	}

	hash := HashLineItems(qboLines)

	return CalculationResult{
		Lines:       qboLines,
		Total:       total,
		LineHash:    hash,
		IsZeroTotal: total == 0,
	}
}

// scheduleMap indexes dues schedule rows by Level for fast lookup.
type scheduleMap map[string][]airtable.DuesScheduleRow

// buildScheduleMap returns a per-level index with conflict detection.
// Fiscal-year-specific rows take precedence over ALL rows per level.
func buildScheduleMap(rows []airtable.DuesScheduleRow, airtableFY string) (scheduleMap, SkipReason, string) {
	fyRows := make(map[string][]airtable.DuesScheduleRow)
	allRows := make(map[string][]airtable.DuesScheduleRow)

	for _, r := range rows {
		if r.FiscalYear == "ALL" {
			allRows[r.Level] = append(allRows[r.Level], r)
		} else {
			fyRows[r.Level] = append(fyRows[r.Level], r)
		}
	}

	// Detect conflicts within fiscal-year-specific rows.
	for level, levelRows := range fyRows {
		if len(levelRows) > 1 {
			return nil, SkipReasonDuesScheduleConflict,
				fmt.Sprintf("multiple dues schedule rows for fiscal year %s level %q", airtableFY, level)
		}
	}
	// Detect conflicts within ALL rows.
	for level, levelRows := range allRows {
		if len(levelRows) > 1 {
			return nil, SkipReasonDuesScheduleConflict,
				fmt.Sprintf("multiple ALL dues schedule rows for level %q", level)
		}
	}

	// Merge: fiscal-year-specific wins over ALL.
	sm := make(scheduleMap)
	for level, levelRows := range allRows {
		sm[level] = levelRows
	}
	for level, levelRows := range fyRows {
		sm[level] = levelRows // overwrite ALL if present
	}

	return sm, SkipReasonNone, ""
}

// resolve returns the single schedule row for a level, or false if not found.
func (sm scheduleMap) resolve(level string) (airtable.DuesScheduleRow, bool) {
	rows, ok := sm[level]
	if !ok || len(rows) == 0 {
		return airtable.DuesScheduleRow{}, false
	}
	return rows[0], true
}
