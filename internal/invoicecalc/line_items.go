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
	Qty         float64 // 0 means default (1)
	UnitPrice   float64 // 0 means default (Amount)
}

func (l lineItem) toQBOLine() qbo.InvoiceLine {
	qty := l.Qty
	if qty == 0 {
		qty = 1
	}
	unitPrice := l.UnitPrice
	if unitPrice == 0 {
		unitPrice = l.Amount
	}
	return qbo.InvoiceLine{
		Amount:      l.Amount,
		DetailType:  "SalesItemLineDetail",
		Description: l.Description,
		SalesItemLineDetail: &qbo.SalesItemLineDetail{
			ItemRef:   qbo.ItemRef{Value: l.ItemID},
			Qty:       qty,
			UnitPrice: unitPrice,
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
	switch {
	case input.Member.IntlLife:
		if items.InternationalLifeMember == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		lines = append(lines, lineItem{ItemID: items.InternationalLifeMember, Description: "International Life Membership", Amount: -intl.Amount})
	case input.Member.RecentMSP:
		if items.InternationalMSP == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		lines = append(lines, lineItem{ItemID: items.InternationalMSP, Description: "Recent MSP (International)", Amount: -intl.Amount})
	}
	if input.FY.IsLate(input.AsOf) {
		if fee, ok := schedule.resolve("International Late Fee"); ok {
			lines = append(lines, lineItem{ItemID: items.InternationalLateFee, Description: "International Late Fee", Amount: fee.Amount})
		}
	}
	// International Reinstatement — additive for Reclaimable members only.
	// Reclaimable and IntlLife are mutually exclusive by definition.
	if input.Member.Reclaimable {
		if items.InternationalReinstatement == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		if reinstatement, ok := schedule.resolve("International Reinstatement"); ok {
			lines = append(lines, lineItem{ItemID: items.InternationalReinstatement, Description: "International Reinstatement", Amount: reinstatement.Amount})
		}
	}

	// District
	dist, ok := schedule.resolve("District")
	if !ok {
		return nil, SkipReasonDuesScheduleMissing
	}
	lines = append(lines, lineItem{ItemID: items.District, Description: "District Dues", Amount: dist.Amount})
	switch {
	case input.Member.DistrictLife:
		if items.DistrictLifeMember == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		lines = append(lines, lineItem{ItemID: items.DistrictLifeMember, Description: "District Life Membership", Amount: -dist.Amount})
	case input.Member.RecentMSP:
		if items.DistrictMSP == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		lines = append(lines, lineItem{ItemID: items.DistrictMSP, Description: "Recent MSP (District)", Amount: -dist.Amount})
	}
	if input.FY.IsLate(input.AsOf) {
		if fee, ok := schedule.resolve("District Late Fee"); ok {
			lines = append(lines, lineItem{ItemID: items.DistrictLateFee, Description: "District Late Fee", Amount: fee.Amount})
		}
	}

	// State
	state, ok := schedule.resolve("State")
	if !ok {
		return nil, SkipReasonDuesScheduleMissing
	}
	lines = append(lines, lineItem{ItemID: items.State, Description: "State Dues", Amount: state.Amount})
	switch {
	case input.Member.StateLife:
		if items.StateLifeMember == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		lines = append(lines, lineItem{ItemID: items.StateLifeMember, Description: "State Life Membership", Amount: -state.Amount})
	case input.Member.RecentMSP:
		if items.StateMSP == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		lines = append(lines, lineItem{ItemID: items.StateMSP, Description: "Recent MSP (State)", Amount: -state.Amount})
	}
	if input.FY.IsLate(input.AsOf) {
		if fee, ok := schedule.resolve("State Late Fee"); ok {
			lines = append(lines, lineItem{ItemID: items.StateLateFee, Description: "State Late Fee", Amount: fee.Amount})
		}
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
		if input.Member.Retired {
			retiree, ok := schedule.resolve("Local (Retiree)")
			if !ok {
				return nil, SkipReasonDuesScheduleMissing
			}
			if items.BasileusEmeritusOffsetRetiree == "" {
				return nil, SkipReasonQBOItemMappingMissing
			}
			return []lineItem{
				{ItemID: items.LocalRetiree, Description: "Local Dues (Retiree)", Amount: retiree.Amount},
				{ItemID: items.BasileusEmeritusOffsetRetiree, Description: "Basileus Emeritus (Retiree)", Amount: -retiree.Amount},
			}, SkipReasonNone
		}
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
		if input.Member.Retired {
			retiree, ok := schedule.resolve("Local (Retiree)")
			if !ok {
				return nil, SkipReasonDuesScheduleMissing
			}
			if items.LocalLifeMemberRetiree == "" {
				return nil, SkipReasonQBOItemMappingMissing
			}
			return []lineItem{
				{ItemID: items.LocalRetiree, Description: "Local Dues (Retiree)", Amount: retiree.Amount},
				{ItemID: items.LocalLifeMemberRetiree, Description: "Local Life Membership (Retiree)", Amount: -retiree.Amount},
			}, SkipReasonNone
		}
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

	case input.Member.RecentMSP:
		if input.Member.Retired {
			retiree, ok := schedule.resolve("Local (Retiree)")
			if !ok {
				return nil, SkipReasonDuesScheduleMissing
			}
			if items.LocalMSPRetiree == "" {
				return nil, SkipReasonQBOItemMappingMissing
			}
			return []lineItem{
				{ItemID: items.LocalRetiree, Description: "Local Dues (Retiree)", Amount: retiree.Amount},
				{ItemID: items.LocalMSPRetiree, Description: "Recent MSP (Local, Retiree)", Amount: -retiree.Amount},
			}, SkipReasonNone
		}
		local, ok := schedule.resolve("Local")
		if !ok {
			return nil, SkipReasonDuesScheduleMissing
		}
		if items.LocalMSP == "" {
			return nil, SkipReasonQBOItemMappingMissing
		}
		return []lineItem{
			{ItemID: items.Local, Description: "Local Dues", Amount: local.Amount},
			{ItemID: items.LocalMSP, Description: "Recent MSP (Local)", Amount: -local.Amount},
		}, SkipReasonNone

	case input.Member.Retired:
		retiree, ok := schedule.resolve("Local (Retiree)")
		if !ok {
			return nil, SkipReasonDuesScheduleMissing
		}
		lines := []lineItem{{ItemID: items.LocalRetiree, Description: "Local Dues (Retiree)", Amount: retiree.Amount}}
		if input.PollWorkerCredit > 0 {
			lines = append(lines, pollWorkerCreditLine(items, input.PollWorkerCredit, retiree.Amount))
		}
		return lines, SkipReasonNone

	default:
		local, ok := schedule.resolve("Local")
		if !ok {
			return nil, SkipReasonDuesScheduleMissing
		}
		lines := []lineItem{{ItemID: items.Local, Description: "Local Dues", Amount: local.Amount}}
		if input.PollWorkerCredit > 0 {
			lines = append(lines, pollWorkerCreditLine(items, input.PollWorkerCredit, local.Amount))
		}
		return lines, SkipReasonNone
	}
}

// pollWorkerCreditLine builds the poll worker credit line item with fractional qty.
// The QBO product has a fixed unit price (default $50); qty = credit_applied / unit_price.
func pollWorkerCreditLine(items config.QBOItemsConfig, credit, localAmount float64) lineItem {
	unitPrice := items.PollWorkerCreditUnit
	if unitPrice <= 0 {
		unitPrice = 50.0
	}
	applied := min(credit, localAmount)
	qty := applied / unitPrice
	return lineItem{
		ItemID:      items.PollWorkerCredit,
		Description: "Poll Worker Credit",
		Amount:      -applied,
		Qty:         qty,
		UnitPrice:   -unitPrice,
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
