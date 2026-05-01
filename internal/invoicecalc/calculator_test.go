package invoicecalc

import (
	"testing"
	"time"

	"github.com/willmadison/invoicer/internal/airtable"
	"github.com/willmadison/invoicer/internal/config"
	"github.com/willmadison/invoicer/internal/fiscal"
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
	LocalLifeMember:         "ITEM_LOCAL_LIFE",
	BasileusEmeritusOffset:  "ITEM_BE",
	PollWorkerCredit:        "ITEM_PWC",
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
	// BE wins: Local + offset = $0, so total = Intl + Dist + State
	want := 125.00 + 20.00 + 15.00
	if result.Total != want {
		t.Errorf("BE total: got %.2f, want %.2f", result.Total, want)
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
