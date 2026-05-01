package invoicecalc

import (
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// lineItem is an intermediate representation before becoming a QBO InvoiceLine.
type lineItem struct {
	ItemID      string
	Description string
	Amount      float64 // negative for credits
}

func (l lineItem) toQBOLine() qbo.InvoiceLine {
	return qbo.InvoiceLine{
		Amount:      l.Amount,
		DetailType:  "SalesItemLineDetail",
		Description: l.Description,
		SalesItemLineDetail: &qbo.SalesItemLineDetail{
			ItemRef:   qbo.ItemRef{Value: l.ItemID},
			Qty:       1,
			UnitPrice: l.Amount,
		},
	}
}

// buildLineItems constructs the full set of invoice line items for a member.
// It applies life membership offsets, BE offsets, retiree rate, and poll worker credits.
func buildLineItems(input CalculationInput, schedule scheduleMap, items config.QBOItemsConfig) ([]lineItem, SkipReason) {
	var lines []lineItem

	// International
	intl, ok := schedule.resolve("International")
	if !ok {
		return nil, SkipReasonDuesScheduleMissing
	}
	lines = append(lines, lineItem{ItemID: items.International, Description: "International Dues", Amount: intl.Amount})
	if input.Member.IntlLife {
		if items.InternationalLifeMember == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		lines = append(lines, lineItem{ItemID: items.InternationalLifeMember, Description: "International Life Membership", Amount: -intl.Amount})
	}

	// District
	dist, ok := schedule.resolve("District")
	if !ok {
		return nil, SkipReasonDuesScheduleMissing
	}
	lines = append(lines, lineItem{ItemID: items.District, Description: "District Dues", Amount: dist.Amount})
	if input.Member.DistrictLife {
		if items.DistrictLifeMember == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		lines = append(lines, lineItem{ItemID: items.DistrictLifeMember, Description: "District Life Membership", Amount: -dist.Amount})
	}

	// State
	state, ok := schedule.resolve("State")
	if !ok {
		return nil, SkipReasonDuesScheduleMissing
	}
	lines = append(lines, lineItem{ItemID: items.State, Description: "State Dues", Amount: state.Amount})
	if input.Member.StateLife {
		if items.StateLifeMember == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		lines = append(lines, lineItem{ItemID: items.StateLifeMember, Description: "State Life Membership", Amount: -state.Amount})
	}

	// Local — priority: BE > LocalLife > Retired > Standard
	localLines, skip := buildLocalLines(input, schedule, items)
	if skip != SkipReasonNone {
		return nil, skip
	}
	lines = append(lines, localLines...)

	// Late fee
	if input.FY.IsLate(input.AsOf) {
		if lateFee, ok := schedule.resolve("Local Late Fee"); ok {
			lines = append(lines, lineItem{ItemID: items.LocalLateFee, Description: "Local Late Fee", Amount: lateFee.Amount})
		}
	}

	return lines, SkipReasonNone
}

func buildLocalLines(input CalculationInput, schedule scheduleMap, items config.QBOItemsConfig) ([]lineItem, SkipReason) {
	switch {
	case input.Member.BasileusEmeritus:
		local, ok := schedule.resolve("Local")
		if !ok {
			return nil, SkipReasonDuesScheduleMissing
		}
		if items.BasileusEmeritusOffset == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		return []lineItem{
			{ItemID: items.Local, Description: "Local Dues", Amount: local.Amount},
			{ItemID: items.BasileusEmeritusOffset, Description: "Basileus Emeritus", Amount: -local.Amount},
		}, SkipReasonNone

	case input.Member.LocalLife:
		local, ok := schedule.resolve("Local")
		if !ok {
			return nil, SkipReasonDuesScheduleMissing
		}
		if items.LocalLifeMember == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		return []lineItem{
			{ItemID: items.Local, Description: "Local Dues", Amount: local.Amount},
			{ItemID: items.LocalLifeMember, Description: "Local Life Membership", Amount: -local.Amount},
		}, SkipReasonNone

	case input.Member.Retired:
		retiree, ok := schedule.resolve("Local (Retiree)")
		if !ok {
			return nil, SkipReasonDuesScheduleMissing
		}
		// Apply poll worker credits against retiree rate, floor at $0.
		amount := retiree.Amount - input.PollWorkerCredit
		if amount < 0 {
			amount = 0
		}
		lines := []lineItem{{ItemID: items.LocalRetiree, Description: "Local Dues (Retiree)", Amount: retiree.Amount}}
		if input.PollWorkerCredit > 0 {
			lines = append(lines, lineItem{ItemID: items.PollWorkerCredit, Description: "Poll Worker Credit", Amount: -min(input.PollWorkerCredit, retiree.Amount)})
		}
		_ = amount
		return lines, SkipReasonNone

	default:
		local, ok := schedule.resolve("Local")
		if !ok {
			return nil, SkipReasonDuesScheduleMissing
		}
		lines := []lineItem{{ItemID: items.Local, Description: "Local Dues", Amount: local.Amount}}
		if input.PollWorkerCredit > 0 {
			credit := min(input.PollWorkerCredit, local.Amount)
			lines = append(lines, lineItem{ItemID: items.PollWorkerCredit, Description: "Poll Worker Credit", Amount: -credit})
		}
		return lines, SkipReasonNone
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
