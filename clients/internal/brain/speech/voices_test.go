package speech

import (
	"os"
	"path/filepath"
)

import "testing"

/*
 * "A woman's voice or a man's" is the question people actually have.
 *
 * "alba, amy, lessac, northern_english_male" is not an answer to it; it is a
 * list of names that has to be researched before it can be chosen from.
 */
func TestPickingAVoiceByKind(t *testing.T) {
	// The names carry no pattern, which is why this is a list and not a rule.
	women := []string{"en_GB-alba-medium", "en_US-amy-medium", "it_IT-paola-medium"}
	men := []string{"en_US-lessac-medium", "en_GB-northern_english_male-medium",
		"de_DE-thorsten-medium", "bg_BG-dimitar-medium"}

	for _, id := range women {
		if voiceSex[id] != "woman" {
			t.Errorf("%s is not marked as a woman's voice", id)
		}
	}

	for _, id := range men {
		if voiceSex[id] != "man" {
			t.Errorf("%s is not marked as a man's voice", id)
		}
	}

	// Nothing is guessed at: an unknown voice is offered without a description
	// rather than assigned one.
	if voiceSex["xx_XX-unknown-medium"] != "" {
		t.Error("a voice nobody has described was given a kind anyway")
	}
}

/*
 * A piper reached through a symlink still has its voices found.
 *
 * Unpacking piper into ~/.local/share/piper and linking it from ~/.local/bin
 * is the ordinary way to install it — it is what this program's own installer
 * does — and the voices sit beside the target, not beside the link. Searching
 * only beside the link found nothing, so the neural voice was installed,
 * reported as installed, and unreachable: every answer came out in the robotic
 * fallback with nothing anywhere explaining why.
 */
func TestVoicesAreFoundThroughASymlink(t *testing.T) {
	real := t.TempDir()
	linked := t.TempDir()

	// A piper installation: the binary, with its voices in a folder beside it.
	binary := filepath.Join(real, "piper")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	voices := filepath.Join(real, "voices")
	if err := os.MkdirAll(voices, 0o755); err != nil {
		t.Fatal(err)
	}

	model := filepath.Join(voices, "en_GB-test-medium.onnx")
	for _, f := range []string{model, model + ".json"} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// And the link somebody actually runs.
	link := filepath.Join(linked, "piper")
	if err := os.Symlink(binary, link); err != nil {
		t.Skipf("cannot make symlinks here: %v", err)
	}

	found := piperVoiceFiles(&Piper{Binary: link})

	var got bool

	for _, f := range found {
		if filepath.Base(f) == "en_GB-test-medium.onnx" {
			got = true
		}
	}

	if !got {
		t.Errorf("the voice beside the real binary was not found through the link; looked at %v", found)
	}
}
