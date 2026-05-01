package invoicecalc

import (
	"testing"
	"time"

	"github.com/OPPF-IHQ-IT/invoicer/internal/airtable"
	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/fiscal"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

var testFY, _ = fiscal.Parse("FY2026")

var testAsOf = time.Date(2025, 11, 15, 0, 0, 0, 0, time.UTC)
var testAsOfLate = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

var testSchedule = []airtable.DuesScheduleRow{
	{FiscalYear: "2025-2026", Level: "International", Amount: 125.00},
	{FiscalYear: "2025-2026", Level: "District", Amount: 20.00},
	{FiscalYear: "2025-2026", Level: "State", Amount: 15.00},
	{FiscalYear: "2025-2026", Level: "Local", Amount: 198.00},
	{FiscalYear: "2025-2026", Level: "Local (Retiree)", Amount: 98.00},
	{FiscalYear: "2025-2026", Level: "Local Late Fee", Amount: 20.00},
}

var testItems = config.QBOItemsConfig{
	International:           "ITEM_INTL",
	District:                "ITEM_DIST",
	State:                   "ITEM_STATE",
	Local:                   "ITEM_LOCAL",
	LocalRetiree:            "ITEM_LOCAL_RET",
	LocalLateFee:            "ITEM_LATE_FEE",
	InternationalLifeMember: "ITEM_INTL_LIFE",
	DistrictLifeMember:      "ITEM_DIST_LIFE",
	StateLifeMember:         "ITEM_STATE_LIFE",
	LocalLifeMember:            "ITEM_LOCAL_LIFE",
	LocalLifeMemberRetiree:     "ITEM_LOCAL_LIFE_RET",
	BasileusEmeritusOffset:        "ITEM_BE",
	BasileusEmeritusOffsetRetiree: "ITEM_BE_RET",
	InternationalMSP:              "ITEM_INTL_MSP",
	DistrictMSP:                   "ITEM_DIST_MSP",
	StateMSP:                      "ITEM_STATE_MSP",
	LocalMSP:                      "ITEM_LOCAL_MSP",
	LocalMSPRetiree:               "ITEM_LOCAL_MSP_RET",
	PollWorkerCredit:           "ITEM_PWC",
	InternationalReinstatement: "ITEM_REINSTATEMENT",
}

func activeMember() airtable.Member {
	return airtable.Member{RecordID: "rec1", ControlNumber: "001", Email: "test@example.com", Status: "Invoicable"}
}

func TestCalculate_StandardActiveMember(t *testing.T) {
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: activeMember(), Schedule: testSchedule, Items: testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	want := 125.00 + 20.00 + 15.00 + 198.00
	if result.Total != want {
		t.Errorf("Total: got %.2f, want %.2f", result.Total, want)
	}
}

func TestCalculate_LateFeeApplied(t *testing.T) {
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOfLate, Member: activeMember(), Schedule: testSchedule, Items: testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	want := 125.00 + 20.00 + 15.00 + 198.00 + 20.00
	if result.Total != want {
		t.Errorf("Total with late fee: got %.2f, want %.2f", result.Total, want)
	}
}

func TestCalculate_RetiredMember(t *testing.T) {
	m := activeMember()
	m.Retired = true
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: m, Schedule: testSchedule, Items: testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	want := 125.00 + 20.00 + 15.00 + 98.00
	if result.Total != want {
		t.Errorf("Retired total: got %.2f, want %.2f", result.Total, want)
	}
}

func TestCalculate_BasileusEmeritus_BeatsRetired(t *testing.T) {
	m := activeMember()
	m.Retired = true
	m.BasileusEmeritus = true
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: m, Schedule: testSchedule, Items: testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	// BE wins: Local (Retiree) + retiree offset = $0, so total = Intl + Dist + State
	want := 125.00 + 20.00 + 15.00
	if result.Total != want {
		t.Errorf("BE total: got %.2f, want %.2f", result.Total, want)
	}
	hasRetireeBEOffset := false
	for _, l := range result.Lines {
		if l.SalesItemLineDetail != nil && l.SalesItemLineDetail.ItemRef.Value == "ITEM_BE_RET" {
			hasRetireeBEOffset = true
		}
	}
	if !hasRetireeBEOffset {
		t.Error("expected ITEM_BE_RET line item, not found")
	}
}

func TestCalculate_QuadrupleLifeMember(t *testing.T) {
	m := activeMember()
	m.IntlLife = true
	m.DistrictLife = true
	m.StateLife = true
	m.LocalLife = true
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: m, Schedule: testSchedule, Items: testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	if result.Total != 0 {
		t.Errorf("Quadruple life total: got %.2f, want 0.00", result.Total)
	}
	if !result.IsZeroTotal {
		t.Error("IsZeroTotal should be true for quadruple life member")
	}
}

func TestCalculate_RecentMSPMember(t *testing.T) {
	m := activeMember()
	m.RecentMSP = true
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: m, Schedule: testSchedule, Items: testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	// All four levels offset to $0.
	if result.Total != 0 {
		t.Errorf("MSP total: got %.2f, want 0.00", result.Total)
	}
	wantItems := []string{"ITEM_INTL_MSP", "ITEM_DIST_MSP", "ITEM_STATE_MSP", "ITEM_LOCAL_MSP"}
	found := make(map[string]bool)
	for _, l := range result.Lines {
		if l.SalesItemLineDetail != nil {
			found[l.SalesItemLineDetail.ItemRef.Value] = true
		}
	}
	for _, id := range wantItems {
		if !found[id] {
			t.Errorf("expected MSP offset line item %s, not found", id)
		}
	}
}

func TestCalculate_RecentMSPRetiredMember(t *testing.T) {
	m := activeMember()
	m.RecentMSP = true
	m.Retired = true
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: m, Schedule: testSchedule, Items: testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	if result.Total != 0 {
		t.Errorf("MSP retired total: got %.2f, want 0.00", result.Total)
	}
	hasRetireeLocalMSP := false
	for _, l := range result.Lines {
		if l.SalesItemLineDetail != nil && l.SalesItemLineDetail.ItemRef.Value == "ITEM_LOCAL_MSP_RET" {
			hasRetireeLocalMSP = true
		}
	}
	if !hasRetireeLocalMSP {
		t.Error("expected ITEM_LOCAL_MSP_RET line item, not found")
	}
}

func TestCalculate_LocalLifeRetiredMember(t *testing.T) {
	m := activeMember()
	m.LocalLife = true
	m.Retired = true
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: m, Schedule: testSchedule, Items: testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	// Retiree rate ($98) with life offset — net local = $0.
	want := 125.00 + 20.00 + 15.00
	if result.Total != want {
		t.Errorf("local life (retiree) total: got %.2f, want %.2f", result.Total, want)
	}
	// Verify the retiree life item is used, not the standard life item.
	hasRetireeTier := false
	for _, l := range result.Lines {
		if l.SalesItemLineDetail != nil && l.SalesItemLineDetail.ItemRef.Value == "ITEM_LOCAL_LIFE_RET" {
			hasRetireeTier = true
		}
	}
	if !hasRetireeTier {
		t.Error("expected ITEM_LOCAL_LIFE_RET line item, not found")
	}
}

func TestCalculate_PollWorkerCreditReducesLocal(t *testing.T) {
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: activeMember(),
		PollWorkerCredit: 50.00,
		Schedule:         testSchedule,
		Items:            testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	want := 125.00 + 20.00 + 15.00 + 198.00 - 50.00
	if result.Total != want {
		t.Errorf("Poll worker credit total: got %.2f, want %.2f", result.Total, want)
	}
}

func TestCalculate_PollWorkerCreditFloorsAtZero(t *testing.T) {
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: activeMember(),
		PollWorkerCredit: 300.00, // more than local dues
		Schedule:         testSchedule,
		Items:            testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	// Credit capped at local amount ($198), not allowed to go negative
	want := 125.00 + 20.00 + 15.00 // local = 0
	if result.Total != want {
		t.Errorf("Credit floor total: got %.2f, want %.2f", result.Total, want)
	}
}

func TestCalculate_PollWorkerCreditFractionalQty(t *testing.T) {
	items := testItems
	items.PollWorkerCreditUnit = 50.0
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: activeMember(),
		PollWorkerCredit: 75.00, // 1.5 units at $50
		Schedule:         testSchedule,
		Items:            items,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	want := 125.00 + 20.00 + 15.00 + 198.00 - 75.00
	if result.Total != want {
		t.Errorf("fractional credit total: got %.2f, want %.2f", result.Total, want)
	}
	// Find the poll worker credit line and verify qty=1.5.
	var pwcLine *qbo.InvoiceLine
	for i := range result.Lines {
		if result.Lines[i].SalesItemLineDetail != nil && result.Lines[i].SalesItemLineDetail.ItemRef.Value == "ITEM_PWC" {
			pwcLine = &result.Lines[i]
			break
		}
	}
	if pwcLine == nil {
		t.Fatal("poll worker credit line not found")
	}
	if pwcLine.SalesItemLineDetail.Qty != 1.5 {
		t.Errorf("poll worker credit qty: got %v, want 1.5", pwcLine.SalesItemLineDetail.Qty)
	}
	if pwcLine.SalesItemLineDetail.UnitPrice != -50.0 {
		t.Errorf("poll worker credit unit price: got %v, want -50.0", pwcLine.SalesItemLineDetail.UnitPrice)
	}
}

func TestCalculate_DuesScheduleConflict(t *testing.T) {
	conflictSchedule := append(testSchedule, airtable.DuesScheduleRow{
		FiscalYear: "2025-2026", Level: "Local", Amount: 210.00,
	})
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: activeMember(), Schedule: conflictSchedule, Items: testItems,
	})
	if result.SkipReason != SkipReasonDuesScheduleConflict {
		t.Errorf("expected dues_schedule_conflict, got %q", result.SkipReason)
	}
}

func TestCalculate_ReclaimableMemberOwesReinstatement(t *testing.T) {
	m := activeMember()
	m.Reclaimable = true
	schedule := append(testSchedule, airtable.DuesScheduleRow{
		FiscalYear: "2025-2026", Level: "International Reinstatement", Amount: 3.00,
	})
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: m, Schedule: schedule, Items: testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	want := 125.00 + 20.00 + 15.00 + 198.00 + 3.00
	if result.Total != want {
		t.Errorf("Reclaimable total: got %.2f, want %.2f", result.Total, want)
	}
}

func TestCalculate_ALLFallback(t *testing.T) {
	// Only International in schedule uses ALL; others are FY-specific.
	schedule := []airtable.DuesScheduleRow{
		{FiscalYear: "ALL", Level: "International", Amount: 125.00},
		{FiscalYear: "2025-2026", Level: "District", Amount: 20.00},
		{FiscalYear: "2025-2026", Level: "State", Amount: 15.00},
		{FiscalYear: "2025-2026", Level: "Local", Amount: 198.00},
		{FiscalYear: "2025-2026", Level: "Local (Retiree)", Amount: 98.00},
	}
	result := Calculate(CalculationInput{
		FY: testFY, AsOf: testAsOf, Member: activeMember(), Schedule: schedule, Items: testItems,
	})
	if result.SkipReason != SkipReasonNone {
		t.Fatalf("unexpected skip: %s", result.SkipReason)
	}
	want := 125.00 + 20.00 + 15.00 + 198.00
	if result.Total != want {
		t.Errorf("ALL fallback total: got %.2f, want %.2f", result.Total, want)
	}
}
