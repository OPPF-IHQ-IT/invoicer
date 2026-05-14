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
	SalesTermID  string // QBO Term entity ID; populates the invoice "Terms" dropdown
	Line         []InvoiceLine
}
