package brain

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/voiceprint"
)

/*
 * "Answer only me" switched on in 1.0.0 or 0.13 is switched off on open.
 *
 * 1.0.1 withdrew the model that tells voices apart (trained on recordings
 * offered for research only). Left on, the switch would hold every turn as
 * "cannot recognise your voice", and the brain would answer nobody. Off, it
 * answers any voice, as it did before the feature existed. Turned off once,
 * in the settings file too; everything else stays as it was.
 */
func TestAnswerOnlyMeIsSwitchedOffWhenTheModelIsWithdrawn(t *testing.T) {
	if voiceprint.Offered() {
		t.Skip("a speaker model is offered in this build")
	}

	root := t.TempDir()
	path := filepath.Join(root, "brain.sqlite")

	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	cfg := config.Default()
	cfg.Owner = "Alex"
	cfg.OnlyMe = true
	cfg.VoiceMatch = 0.62
	cfg.CancelRoom = true

	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	b := New(db, cfg, root, path, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if b.Cfg.OnlyMe {
		t.Error("the brain still answers only one voice with no model to tell voices apart")
	}

	saved, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if saved.OnlyMe {
		t.Error("the settings file still has answer only me on")
	}

	if saved.Owner != "Alex" || saved.VoiceMatch != 0.62 || !saved.CancelRoom {
		t.Errorf("other settings moved with it: %+v", saved)
	}
}

// Asked in conversation to answer only one voice, it says why it cannot and
// changes nothing.
func TestAskingToAnswerOnlyMeSaysItIsNotOffered(t *testing.T) {
	if voiceprint.Offered() {
		t.Skip("a speaker model is offered in this build")
	}

	root := t.TempDir()

	b := &Brain{Cfg: config.Default(), Root: root}

	said, err := b.changeSetting(context.Background(), "only_me", true)
	if err != nil {
		t.Fatal(err)
	}

	if b.Cfg.OnlyMe {
		t.Error("answer only me was switched on with no model to tell voices apart")
	}

	if !strings.Contains(said, "not offered") {
		t.Errorf("the answer does not say the feature is not offered: %q", said)
	}

	if strings.ContainsAny(said, "–—") {
		t.Errorf("the answer uses a dash: %q", said)
	}
}
