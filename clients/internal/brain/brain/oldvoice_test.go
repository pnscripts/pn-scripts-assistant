package brain

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/speech"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * A voice chosen in 0.13 that 1.0 withdrew is moved to the default on open.
 *
 * The three voices 0.13 installed were fine-tuned from recordings licensed for
 * research only, and 1.0 no longer speaks with them. Somebody who had picked
 * one of them would otherwise keep a setting naming a voice that is never
 * used, and the voice list would show nothing chosen. The setting becomes the
 * robot (the default, now made of Cori's voice) in the settings file too, once;
 * any other choice is left as it was.
 */
func TestAVoiceAnOlderReleaseInstalledIsMovedToTheDefaultOnOpen(t *testing.T) {
	for _, tc := range []struct{ had, want string }{
		{"en_GB-alba-medium", speech.RobotID},
		{"en_GB-northern_english_male-medium", speech.RobotID},
		{"en_GB-jenny_dioco-medium", speech.RobotID},
		{speech.ManVoice, speech.ManVoice},
		{"en_US-norman-medium", "en_US-norman-medium"},
	} {
		t.Run(tc.had, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "brain.sqlite")

			db, err := store.Open(path)
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() {
				db.Close()
				speech.SetVoice("")
			})

			cfg := config.Default()
			cfg.Owner = "Alex"
			cfg.Voice = tc.had

			if err := cfg.Save(root); err != nil {
				t.Fatal(err)
			}

			b := New(db, cfg, root, path, slog.New(slog.NewTextHandler(io.Discard, nil)))

			if b.Cfg.Voice != tc.want {
				t.Errorf("opened with %q, the brain uses %q, want %q", tc.had, b.Cfg.Voice, tc.want)
			}

			saved, err := config.Load(root)
			if err != nil {
				t.Fatal(err)
			}

			if saved.Voice != tc.want {
				t.Errorf("the settings file says %q, want %q", saved.Voice, tc.want)
			}

			// Nothing else in the settings moved with it.
			if saved.Owner != "Alex" {
				t.Errorf("the owner became %q", saved.Owner)
			}
		})
	}
}
