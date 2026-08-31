package preflight

import "testing"

// Blocking must name what stops the brain, and stay silent about what does not
// — an optional piece listed as blocking would tell somebody to install
// something they do not need before the program will run.
func TestBlockingNamesOnlyWhatStops(t *testing.T) {
	results := []Result{
		{Requirement: Requirement{Name: "ollama"}, State: Missing},
		{Requirement: Requirement{Name: "piper", Optional: true}, State: Missing},
		{Requirement: Requirement{Name: "sqlite"}, State: OK},
	}

	blocking := Blocking(results)

	if len(blocking) != 1 || blocking[0].Requirement.Name != "ollama" {
		t.Fatalf("blocking = %v, want only ollama", blocking)
	}

	if BlockingCount(results) != len(blocking) {
		t.Fatalf("count %d disagrees with the list %d",
			BlockingCount(results), len(blocking))
	}
}
