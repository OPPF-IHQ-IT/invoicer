package campaign

import (
	"os"
	"path/filepath"
	"testing"
)

// writeCSVFixture writes content to a temp file and returns its path.
func writeCSVFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "form.csv")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

// A CSV without the optional Designation column must load fine and leave
// Designation empty — NOT accidentally read column 0 (Timestamp).
func TestLoadRowsWithoutDesignationColumn(t *testing.T) {
	csv := "Timestamp,Full Name,Email Address,Control Number,Invoice Amount,Please enter your requested invoice amount,Consent/Authorization\n" +
		"1/2/2026 10:00:00,Jane Doe,jane@example.com,CT-0001,$150 — Standard,,Yes\n"
	rows, err := LoadRows(writeCSVFixture(t, csv))
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("row count: got %d, want 1", len(rows))
	}
	if rows[0].Designation != "" {
		t.Errorf("missing Designation column should read empty, got %q", rows[0].Designation)
	}
}

// A CSV that includes the Designation column reads its value regardless of
// column position.
func TestLoadRowsWithDesignationColumn(t *testing.T) {
	csv := "Timestamp,Full Name,Email Address,Control Number,Invoice Amount,Please enter your requested invoice amount,Designation,Consent/Authorization\n" +
		"1/2/2026 10:00:00,Jane Doe,jane@example.com,CT-0001,$120 — Cover a month of Facility Internet (Fiber),,Facility Internet (Fiber),Yes\n"
	rows, err := LoadRows(writeCSVFixture(t, csv))
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("row count: got %d, want 1", len(rows))
	}
	if got, want := rows[0].Designation, "Facility Internet (Fiber)"; got != want {
		t.Errorf("Designation: got %q, want %q", got, want)
	}
}
