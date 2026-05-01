package reports

import "fmt"

func printTable(r Report) {
	fmt.Printf("invoicer %s — %s\n", r.Mode, r.FiscalYear)
	fmt.Printf("Period: %s to %s  |  As of: %s\n\n", r.PeriodStart, r.PeriodEnd, r.AsOf)

	printSection("CREATE", r.Created)
	printSection("NO CHANGE", r.NoChange)
	printSection("SEND", r.Sent)
	printSection("SKIPPED", r.Skipped)
	printSection("ERRORS", r.Errors)

	fmt.Printf("\nSummary: %d create  %d no-change  %d send  %d skipped  %d errors\n",
		len(r.Created), len(r.NoChange), len(r.Sent), len(r.Skipped), len(r.Errors))
}

func printSection(label string, entries []MemberEntry) {
	if len(entries) == 0 {
		return
	}
	fmt.Printf("── %s (%d) ──\n", label, len(entries))
	for _, e := range entries {
		fmt.Printf("  %-12s  %-30s  %-30s", e.ControlNumber, e.Name, e.Email)
		if e.Total != 0 {
			fmt.Printf("  $%8.2f", e.Total)
		}
		if e.SkipReason != "" {
			fmt.Printf("  [%s]", e.SkipReason)
		}
		if e.Detail != "" {
			fmt.Printf("  %s", e.Detail)
		}
		fmt.Println()
	}
	fmt.Println()
}
