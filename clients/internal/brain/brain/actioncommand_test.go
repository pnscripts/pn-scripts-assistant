package brain

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/learning"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * What 1.3.0 wrote stays in the data after the fix in the code: every
 * action written in the window left "Command: <the line>" in "What I did",
 * which programs on this computer and paired devices read. Opened by this
 * version, those lines keep the action's name and lose the command, once
 * per brain; lines of every other tool, and the actions themselves, are
 * left exactly as they were.
 */
func TestTheCommandsA130BrainWroteOnTheRecordAreForgottenOnOpen(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "brain.sqlite")

	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		db.Close()
		learning.SetBrainRoot("")
	})

	// As 1.3.0 wrote them (handover.go AddOutsideAction).
	for _, command := range []string{"rsync -a ~/Pictures /mnt/backup-heron", "echo plover-quartz-7731"} {
		if _, err := db.AddDeed(store.Deed{
			Tool:      ActionsTool,
			Summary:   "An action other programs may run was written: backup",
			SummaryBG: "Написано е действие, което другите програми могат да изпълняват: backup",
			Asked:     "Command: " + command,
			Outcome:   "done",
			Changes:   true,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Another tool's line that happens to begin the same way is its own.
	if _, err := db.AddDeed(store.Deed{Tool: "run_command", Summary: "Ran a command", Asked: "Command: ls ~/kingfisher", Outcome: "done"}); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Owner = "Alex"

	New(db, cfg, root, path, slog.New(slog.NewTextHandler(io.Discard, nil)))

	deeds, err := db.Deeds(store.DeedFilter{Limit: 10})
	if err != nil || len(deeds) != 3 {
		t.Fatalf("the record after opening: %d lines, %v", len(deeds), err)
	}

	for _, d := range deeds {
		all := d.Summary + d.SummaryBG + d.Asked + d.Request + d.Result

		switch d.Tool {
		case ActionsTool:
			if strings.Contains(all, "heron") || strings.Contains(all, "plover") || d.Asked != "" {
				t.Errorf("an action's command is still on the record: %+v", d)
			}

			if !strings.Contains(d.Summary, "backup") || d.Outcome != "done" || !d.Changes {
				t.Errorf("the line lost more than the command: %+v", d)
			}
		default:
			if d.Asked != "Command: ls ~/kingfisher" {
				t.Errorf("another tool's line was changed: %+v", d)
			}
		}
	}

	// Once: a line written later is no business of the tidy (and this
	// version writes none like it; the server test holds that).
	if _, err := db.AddDeed(store.Deed{Tool: ActionsTool, Summary: "later", Asked: "Command: true", Outcome: "done"}); err != nil {
		t.Fatal(err)
	}

	New(db, cfg, root, path, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if later, _ := db.Deeds(store.DeedFilter{Limit: 1}); len(later) != 1 || later[0].Asked != "Command: true" {
		t.Errorf("the tidy ran again on the next open: %+v", later)
	}
}
