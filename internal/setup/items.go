package setup

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/OPPF-IHQ-IT/invoicer/internal/config"
	"github.com/OPPF-IHQ-IT/invoicer/internal/qbo"
)

// component describes a single dues line item that needs a QBO product mapped to it.
type component struct {
	Label     string // human-readable prompt label
	ConfigKey string // yaml key under qbo_items
	Optional  bool   // true = press Enter to skip
}

var components = []component{
	{Label: "International Dues", ConfigKey: "international"},
	{Label: "District Dues", ConfigKey: "district"},
	{Label: "State Dues", ConfigKey: "state"},
	{Label: "Local Dues", ConfigKey: "local"},
	{Label: "Local Dues (Retiree)", ConfigKey: "local_retiree"},
	{Label: "Local Late Fee", ConfigKey: "local_late_fee"},
	{Label: "International Life Membership offset", ConfigKey: "international_life_membership", Optional: true},
	{Label: "District Life Membership offset", ConfigKey: "district_life_membership", Optional: true},
	{Label: "State Life Membership offset", ConfigKey: "state_life_membership", Optional: true},
	{Label: "Local Life Membership offset (non-retiree)", ConfigKey: "local_life_membership", Optional: true},
	{Label: "Local Life Membership offset (retiree)", ConfigKey: "local_life_membership_retiree", Optional: true},
	{Label: "Basileus Emeritus offset (non-retiree)", ConfigKey: "basileus_emeritus_offset", Optional: true},
	{Label: "Basileus Emeritus offset (retiree)", ConfigKey: "basileus_emeritus_offset_retiree", Optional: true},
	{Label: "Recent MSP offset — International", ConfigKey: "international_msp", Optional: true},
	{Label: "Recent MSP offset — District", ConfigKey: "district_msp", Optional: true},
	{Label: "Recent MSP offset — State", ConfigKey: "state_msp", Optional: true},
	{Label: "Recent MSP offset — Local (non-retiree)", ConfigKey: "local_msp", Optional: true},
	{Label: "Recent MSP offset — Local (retiree)", ConfigKey: "local_msp_retiree", Optional: true},
	{Label: "Poll Worker Dues Credit", ConfigKey: "poll_worker_credit", Optional: true},
	{Label: "International Reinstatement", ConfigKey: "international_reinstatement", Optional: true},
}

// RunItems fetches QBO products and walks through mapping each dues component interactively.
func RunItems(cfg *config.Config) error {
	configPath := filepath.Join(mustHomeDir(), ".config", "invoicer", "config.yaml")

	qboClient, err := qbo.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("connecting to QBO (have you run 'invoicer auth login'?): %w", err)
	}

	fmt.Println("\nFetching your QBO products...")
	items, err := qboClient.ListItems(context.Background())
	if err != nil {
		return fmt.Errorf("loading QBO items: %w", err)
	}
	if len(items) == 0 {
		return fmt.Errorf("no active items found in QBO — make sure your products are set up")
	}

	fmt.Printf("Found %d products.\n\n", len(items))

	scanner := bufio.NewScanner(os.Stdin)
	selections := make(map[string]string) // configKey → QBO item ID

	for _, comp := range components {
		skipHint := ""
		if comp.Optional {
			skipHint = " (Enter to skip)"
		}
		fmt.Printf("─── %s%s ───\n\n", comp.Label, skipHint)
		for i, item := range items {
			desc := ""
			if item.Description != "" {
				desc = " — " + item.Description
			}
			fmt.Printf("  %2d. %s%s\n", i+1, item.Name, desc)
		}
		fmt.Println()

		for {
			fmt.Printf("Select product for %q [1-%d]%s: ", comp.Label, len(items), skipHint)
			scanner.Scan()
			input := strings.TrimSpace(scanner.Text())

			if input == "" && comp.Optional {
				fmt.Println("  Skipped.")
				break
			}

			n, err := strconv.Atoi(input)
			if err != nil || n < 1 || n > len(items) {
				fmt.Printf("  Please enter a number between 1 and %d.\n", len(items))
				continue
			}

			selected := items[n-1]
			selections[comp.ConfigKey] = selected.ID
			fmt.Printf("  → %s (ID: %s)\n", selected.Name, selected.ID)
			break
		}
		fmt.Println()
	}

	if len(selections) == 0 {
		fmt.Println("No selections made — config not updated.")
		return nil
	}

	if err := applyItemsToConfig(configPath, selections); err != nil {
		return fmt.Errorf("updating config: %w", err)
	}

	fmt.Printf("Config updated at %s\n", configPath)
	fmt.Println("\nRun 'invoicer preview --fiscal-year FY2026' to verify everything looks right.")
	return nil
}

// applyItemsToConfig does targeted line-level replacements for each yaml key,
// preserving comments and all other config values.
func applyItemsToConfig(path string, updates map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	content := string(data)
	for key, id := range updates {
		re := regexp.MustCompile(`(?m)^(\s+` + regexp.QuoteMeta(key) + `:\s*)"[^"]*"`)
		content = re.ReplaceAllString(content, `${1}"`+id+`"`)
	}

	return os.WriteFile(path, []byte(content), 0600)
}
