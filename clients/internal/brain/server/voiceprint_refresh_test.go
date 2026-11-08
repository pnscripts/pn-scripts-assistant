package server

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
)

/*
 * A voiceprint runtime from before an update is refreshed when the server
 * starts, through voiceprint.Refresh (which decides whether there is anything
 * to do and is tested in its own package), and never while looking things up
 * online is switched off.
 */
func TestTheVoiceprintRuntimeIsRefreshedOnlyWhenLookingOnlineIsOn(t *testing.T) {
	for _, on := range []bool{true, false} {
		t.Setenv("HOME", t.TempDir())

		cfg := config.Default()
		cfg.LookOnline = on

		s := &Server{
			brain: &brain.Brain{Cfg: cfg, Root: t.TempDir()},
			log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		}

		calls := 0
		old := refreshRuntime
		refreshRuntime = func(ctx context.Context, say func(string)) (bool, error) {
			calls++

			say("Updating")

			return true, nil
		}

		s.refreshVoiceprint(context.Background())

		refreshRuntime = old

		if want := map[bool]int{true: 1, false: 0}[on]; calls != want {
			t.Errorf("looking online %v: refreshed %d times, want %d", on, calls, want)
		}
	}
}
