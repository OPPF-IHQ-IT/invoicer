package qbo

import "time"

type Item struct {
	ID          string
	Name        string
	Description string
	Type        string
	Active      bool
}

type Customer struct {
	ID            string
	DisplayName   string
	Email         string
	Notes         string // maps to QBO Notes field; used to store Control #
	Active        bool
	Balance       float64
}

type Invoice struct {
	ID          string
	DocNumber   string
	CustomerRef CustomerRef
	TxnDate     time.Time
	DueDate     time.Time
	TotalAmt    float64
	Balance     float64
	EmailStatus string // NotSet, NeedToSend, EmailSent
	PrintStatus string
	PrivateNote string
	Line        []InvoiceLine
	Status      string // Draft, Pending, Voided, Deleted, Synchronized
	SyncToken   string
	BillEmail   string
	// DeliveryError is QBO's delivery failure, e.g. "Undeliverable". A bounced
	// invoice still has EmailStatus EmailSent, so check this as well.
	DeliveryError string
}

type CustomerRef struct {
	Value string
	Name  string
}

type InvoiceLine struct {
	Amount      float64
	DetailType  string
	Description string
	SalesItemLineDetail *SalesItemLineDetail
}

type SalesItemLineDetail struct {
	ItemRef  ItemRef
	Qty      float64
	UnitPrice float64
	// ClassID is the QBO Class entity ID for this line. QBO products can't
	// carry a default class, so it must be set per line; blank leaves the
	// line unclassified ("Not specified" on class reports).
	ClassID string
}

// WithClass returns a copy of lines with classID set on every line that has
// no class yet. A blank classID returns lines unchanged.
func WithClass(lines []InvoiceLine, classID string) []InvoiceLine {
	if classID == "" {
		return lines
	}
	out := make([]InvoiceLine, len(lines))
	for i, l := range lines {
		if l.SalesItemLineDetail != nil && l.SalesItemLineDetail.ClassID == "" {
			d := *l.SalesItemLineDetail
			d.ClassID = classID
			l.SalesItemLineDetail = &d
		}
		out[i] = l
	}
	return out
}

type ItemRef struct {
	Value string
	Name  string
}

type InvoiceCreateRequest struct {
	CustomerRef  CustomerRef
	DocNumber    string
	TxnDate      string
	DueDate      string
	PrivateNote  string
	CustomerMemo string
	BillEmail    string // populates Invoice.BillEmail; required for /send to deliver without sendTo
	BillEmailCc  string // defaults to the company's Sales form CC setting when both copies are blank
	BillEmailBcc string // defaults to the company's Sales form BCC / "email me a copy" setting
	SalesTermID  string // QBO Term entity ID; populates the invoice "Terms" dropdown
	Line         []InvoiceLine
}
