package autonomy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Only goal mode's own downloads, unused for the time and not in use, are
// removed; a failed removal stays recorded and is tried again.
func TestOnlyItsOwnUnusedDownloadsAreRemoved(t *testing.T) {
	root := t.TempDir()
	ledger := LedgerAt(root)

	now := time.Now()
	old := now.Add(-40 * 24 * time.Hour)

	for _, e := range []Entry{
		{Kind: "model", Name: "qwen3:8b", Bytes: 5 << 30, When: old, Used: old},
		{Kind: "model", Name: "llama3.2:3b", When: old, Used: old},                    // in use
		{Kind: "model", Name: "gemma3:12b", When: old, Used: now.Add(-2 * time.Hour)}, // used lately
		{Kind: "tool", ID: "godot", Name: "Godot", When: old, Used: old},
		{Kind: "integration", ID: "notes", Name: "notes", When: old, Used: old},
		{Kind: "tool", ID: "uv", Name: "uv (Python tools)", When: old, Used: old}, // fails
	} {
		if err := ledger.Add(e); err != nil {
			t.Fatal(err)
		}
	}

	var gone []string

	rm := Removers{
		Model: func(_ context.Context, name string) error { gone = append(gone, "model "+name); return nil },
		Tool: func(id, _ string) error {
			if id == "uv" {
				return errors.New("still here")
			}

			gone = append(gone, "tool "+id)

			return nil
		},
		Integration: func(id string) error { gone = append(gone, "integration "+id); return nil },
	}

	inUse := func(e Entry) bool { return e.Name == "llama3.2:3b" }

	got := Tidy(context.Background(), ledger, UnusedFor, inUse, rm, now)

	if strings.Join(gone, ",") != "model qwen3:8b,tool godot,integration notes" {
		t.Errorf("removed %v", gone)
	}

	if len(got.Failed) != 1 || !strings.Contains(got.Failed[0], "uv") || got.Freed != 5<<30 {
		t.Errorf("tidied %+v", got)
	}

	var left []string
	for _, e := range ledger.Entries() {
		left = append(left, e.Name)
	}

	if strings.Join(left, ",") != "llama3.2:3b,gemma3:12b,uv (Python tools)" {
		t.Errorf("still recorded: %v", left)
	}

	// Something it never put here is never offered for removal.
	if ledger.Mine("model", "mistral:7b") {
		t.Error("a model the owner installed counts as goal mode's")
	}
}
