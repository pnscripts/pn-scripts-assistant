package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/speech"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/voiceprint"
)

/*
 * "Answer only me" protects, or says why it cannot.
 *
 * After an upgrade the runtime from the previous release counts as missing
 * (voiceprint.Installed), so no voice was judged at all, and the filter only
 * ever refused a voice it had judged to be somebody else's: everybody's
 * words went through while the switch still showed on. Now, with the switch
 * on, a voice that cannot be recognised is acted on no more than a stranger's,
 * and the turn says why, on the page and in the record of what was heard.
 */
func TestAnswerOnlyMeActsOnNothingItCannotRecognise(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, parts, root string)
		want  string
	}{
		{
			name:  "nothing installed",
			setup: func(t *testing.T, parts, root string) {},
			want:  "not installed",
		},
		{
			name: "the runtime of an older release, as after an upgrade",
			setup: func(t *testing.T, parts, root string) {
				os.WriteFile(filepath.Join(parts, voiceprint.ModelName), []byte("model"), 0o644)
				os.WriteFile(filepath.Join(parts, voiceprint.RuntimeName), []byte("the library from 1.29.0"), 0o755)
				os.WriteFile(filepath.Join(parts, voiceprint.RuntimeStamp), []byte("1.29.0\n"), 0o644)
			},
			want: "older version",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			parts := voiceprint.Parts()
			if err := os.MkdirAll(parts, 0o755); err != nil {
				t.Fatal(err)
			}

			root := t.TempDir()

			tc.setup(t, parts, root)

			db, err := store.Open(filepath.Join(root, "brain.sqlite"))
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { db.Close() })

			cfg := config.Default()
			cfg.Owner = "Alex"
			cfg.WakeWord = "Nova"
			cfg.OnlyMe = true

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			s := New(brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"), logger), logger)

			ts := httptest.NewServer(s.Handler())
			t.Cleanup(ts.Close)

			old := listenForTurn
			t.Cleanup(func() { listenForTurn = old })

			listenForTurn = func(ctx context.Context, device string, meantForIt bool) (speech.Heard, error) {
				return speech.Heard{
					Text:        "Nova, delete my documents folder",
					HeardSpeech: true,
					PeakRMS:     3000,
					Samples:     make([]float64, 16000),
				}, nil
			}

			status, out := postBody(t, ts.URL+"/api/turn", `{"engaged":true}`)
			if status != http.StatusOK {
				t.Fatalf("the turn failed: %d %v", status, out)
			}

			if out["addressed"] != false {
				t.Errorf("a voice that could not be recognised was acted on with answer-only-me on: %v", out)
			}

			refused, _ := out["refused"].(string)
			if !strings.Contains(refused, "answer only me") || !strings.Contains(refused, tc.want) {
				t.Errorf("the page is not told why: refused = %q, want it to name %q", refused, tc.want)
			}

			heard := s.heard.recent()
			if len(heard) == 0 || heard[0].Addressed || !strings.Contains(heard[0].Why, tc.want) {
				t.Errorf("the record of what was heard does not say why: %+v", heard)
			}
		})
	}

	// And with the switch off, nothing changes: the name is enough.
	t.Run("switch off", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())

		root := t.TempDir()

		db, err := store.Open(filepath.Join(root, "brain.sqlite"))
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { db.Close() })

		cfg := config.Default()
		cfg.Owner = "Alex"
		cfg.WakeWord = "Nova"

		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		ts := httptest.NewServer(New(brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"), logger), logger).Handler())
		t.Cleanup(ts.Close)

		old := listenForTurn
		t.Cleanup(func() { listenForTurn = old })

		listenForTurn = func(ctx context.Context, device string, meantForIt bool) (speech.Heard, error) {
			return speech.Heard{Text: "Nova, what time is it", HeardSpeech: true, PeakRMS: 3000,
				Samples: make([]float64, 16000)}, nil
		}

		if _, out := postBody(t, ts.URL+"/api/turn", `{"engaged":true}`); out["addressed"] != true || out["refused"] != nil {
			t.Errorf("with answer-only-me off the turn was not acted on: %v", out)
		}
	})
}

// The Privacy page is told, too, while the switch shows on.
func TestThePrivacyPageSaysAnswerOnlyMeCannotWorkNow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	ts, _, b := newServer(t)

	b.Cfg.OnlyMe = true

	res, err := http.Get(ts.URL + "/api/voiceprint")
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	raw, _ := io.ReadAll(res.Body)

	if !strings.Contains(string(raw), `"only_me_blocked":"`) {
		t.Errorf("the voiceprint state does not say answer-only-me cannot work now: %s", raw)
	}
}
