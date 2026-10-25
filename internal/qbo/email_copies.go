package qbo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// emailCopies holds the CC and BCC addresses QBO's own invoice form would add.
type emailCopies struct {
	cc  string
	bcc string
}

// salesEmailCopies reads the company's Sales form email settings (Account and
// Settings > Sales > Messages) once per client. Invoices created in the QBO UI
// pick these up automatically, but invoices created through the API do not, so
// without this the KRS/KF never get copies of invoices the invoicer sends.
//
// It fails open: if the settings can't be read, or nothing is configured, no one
// is copied and invoice creation carries on.
func (c *Client) salesEmailCopies(ctx context.Context) emailCopies {
	if c.copies != nil {
		return *c.copies
	}
	copies := c.fetchSalesEmailCopies(ctx)
	c.copies = &copies
	return copies
}

func (c *Client) fetchSalesEmailCopies(ctx context.Context) emailCopies {
	body, err := c.get(ctx, "preferences")
	if err != nil {
		return emailCopies{}
	}
	var prefs struct {
		Preferences struct {
			SalesFormsPrefs struct {
				SalesEmailCc       emailAddr `json:"SalesEmailCc"`
				SalesEmailBcc      emailAddr `json:"SalesEmailBcc"`
				EmailCopyToCompany bool      `json:"EmailCopyToCompany"`
			} `json:"SalesFormsPrefs"`
		} `json:"Preferences"`
	}
	if err := json.Unmarshal(body, &prefs); err != nil {
		return emailCopies{}
	}
	sf := prefs.Preferences.SalesFormsPrefs

	bcc := []string{sf.SalesEmailBcc.Address}
	// "Email me a copy" BCCs the company email address.
	if sf.EmailCopyToCompany {
		bcc = append(bcc, c.companyEmail(ctx))
	}
	return emailCopies{cc: joinAddresses(sf.SalesEmailCc.Address), bcc: joinAddresses(bcc...)}
}

func (c *Client) companyEmail(ctx context.Context) string {
	body, err := c.get(ctx, "companyinfo/"+c.realmID)
	if err != nil {
		return ""
	}
	var info struct {
		CompanyInfo struct {
			Email emailAddr `json:"Email"`
		} `json:"CompanyInfo"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return ""
	}
	return info.CompanyInfo.Email.Address
}

type emailAddr struct {
	Address string `json:"Address"`
}

// joinAddresses drops blanks and duplicates and joins the rest the way QBO
// expects multiple addresses in one field.
func joinAddresses(addrs ...string) string {
	seen := map[string]bool{}
	var out []string
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" || seen[strings.ToLower(a)] {
			continue
		}
		seen[strings.ToLower(a)] = true
		out = append(out, a)
	}
	return strings.Join(out, ", ")
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	u := fmt.Sprintf("%s/v3/company/%s/%s?minorversion=65", c.baseURL(), c.realmID, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	return c.do(req)
}
