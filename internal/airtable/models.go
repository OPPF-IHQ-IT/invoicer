package airtable

import "time"

// Member represents a row from the Members table.
type Member struct {
	RecordID         string
	ControlNumber    string
	Name             string
	Email            string
	Status           string
	QBOCustomerID    string
	IntlLife         bool
	DistrictLife     bool
	StateLife        bool
	LocalLife        bool
	BasileusEmeritus bool
	Retired          bool
	RecentMSP        bool // prepaid for 3 years; Intl/District/State always offset, Local only if ChapterInitiate
	InitiationChapter string
	// ChapterInitiate is true when InitiationChapter matches the configured
	// home chapter. It gates the Recent MSP local dues offset.
	ChapterInitiate bool
	AtLarge          bool // pays International/District/State elsewhere; invoiced for local dues only
	UndergradRate    bool // recent undergraduate alumnus; billed the undergraduate International rate
	// Reclaimable is true when Status == the configured reclaimable status value.
	// These members are invoiceable but also owe an International Reinstatement fee.
	Reclaimable bool

	AddressLine1 string
	AddressLine2 string
	City         string
	State        string
	Zip          string
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

// MemberCredit represents a row from the Member Credits table: a dollar
// credit (overpayment, prepayment, hardship, ...) applied to dues invoices
// until it is spent.
type MemberCredit struct {
	RecordID    string
	MemberID    string // linked Member record ID
	Type        string // e.g. "Overpayment"
	LocalOnly   bool   // Applies To = "Local Only"; otherwise applies to the total due
	Earned      float64
	Spent       float64
	MaxPerYear  float64 // 0 means no cap
	CreatedTime string  // RFC 3339; used for oldest-first ordering
}

// Available returns the unspent credit.
func (c MemberCredit) Available() float64 {
	return c.Earned - c.Spent
}
