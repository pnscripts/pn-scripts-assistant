package server

import "testing"

func TestOnlyReportsAreServedAsReports(t *testing.T) {
	if reportFile("/b", "originality-audit-20261024-060000.md") == "" ||
		reportFile("/b", "originality-audit-20261024-060000-2.csv") == "" {
		t.Error("a report was refused")
	}

	for _, bad := range []string{"../brain.sqlite", "originality-audit-20261024-060000.md/../../x", "vault.json", "originality-audit-1.md"} {
		if reportFile("/b", bad) != "" {
			t.Errorf("%q served", bad)
		}
	}
}
