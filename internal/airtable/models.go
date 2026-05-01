package airtable

import "time"

// Member represents a row from the Members table.
type Member struct {
	RecordID         string
	ControlNumber    string
	Email            string
	Status           string
	QBOCustomerID    string
	IntlLife         bool
	DistrictLife     bool
	StateLife        bool
	LocalLife        bool
	BasileusEmeritus bool
	Retired          bool
	// Reclaimable is true when Status == the configured reclaimable status value.
	// These members are invoiceable but also owe an International Reinstatement fee.
	Reclaimable bool
}

// DuesRecord represents a row from the Dues Records table.
type DuesRecord struct {
	RecordID   string
	FiscalYear string
	MemberID   string // linked Member record ID
	MemberName string
	Email      string
}

// DuesScheduleRow represents a row from the Dues Schedule table.
type DuesScheduleRow struct {
	RecordID   string
	ID         int
	FiscalYear string // e.g. "2025-2026" or "ALL"
	Level      string
	Amount     float64
	DueDate    *time.Time
	Key        string
}

// PollWorkerCredit represents a row from the Poll Worker Dues Credit Utilization table.
type PollWorkerCredit struct {
	RecordID         string
	MemberID         string // linked Member record ID
	FiscalYear       string
	CreditsEarned    float64
	CreditsSpent     float64
	CreditsAvailable float64
}
