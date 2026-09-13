package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Nothing written is the ordinary state, and not an error. A brain with no
// profile is every brain until somebody writes one.
func TestNoProfileIsNotAProblem(t *testing.T) {
	root := t.TempDir()

	text, err := Read(root)
	if err != nil {
		t.Fatalf("reading a profile that does not exist: %v", err)
	}

	if text != "" {
		t.Errorf("read %q from nowhere", text)
	}

	if block := Block(root, 0); block != "" {
		t.Errorf("something was put in the prompt: %q", block)
	}
}

func TestItComesBackAsItWasWritten(t *testing.T) {
	root := t.TempDir()

	const written = "I run a small studio.\n\nI bill in advance and never chase late payments myself."

	if err := Write(root, written); err != nil {
		t.Fatal(err)
	}

	back, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}

	if back != written {
		t.Errorf("came back as %q", back)
	}

	// And it is a file its owner can open, in the brain's own folder, so it
	// travels with the brain rather than with the machine.
	if _, err := os.Stat(filepath.Join(root, FileName)); err != nil {
		t.Errorf("there is no file to edit: %v", err)
	}
}

/*
 * Cleared means gone, not an empty file.
 *
 * Two states that behave the same are two states somebody has to reason about.
 */
func TestClearingItRemovesTheFile(t *testing.T) {
	root := t.TempDir()

	if err := Write(root, "something"); err != nil {
		t.Fatal(err)
	}

	if err := Write(root, "   \n  "); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, FileName)); !os.IsNotExist(err) {
		t.Errorf("the file is still there: %v", err)
	}

	if block := Block(root, 0); block != "" {
		t.Errorf("a cleared profile is still in the prompt: %q", block)
	}
}

/*
 * There is a ceiling, because this is read again on every turn.
 *
 * On a processor managing ten tokens a second, a thousand tokens of profile is
 * a minute and a half of reading before the question has been reached.
 */
func TestThereIsACeiling(t *testing.T) {
	root := t.TempDir()

	if err := Write(root, strings.Repeat("я", MostRunes+500)); err != nil {
		t.Fatal(err)
	}

	back, _ := Read(root)

	// Counted in letters rather than bytes: Cyrillic is two bytes a letter,
	// and cutting by bytes leaves half of one.
	if got := len([]rune(back)); got != MostRunes {
		t.Errorf("kept %d letters, want %d", got, MostRunes)
	}
}

/*
 * A spoken turn gets the opening, cut at a paragraph.
 *
 * Half a sentence about somebody's business is worse than the first paragraph
 * of it, and the whole thing is more silence than anybody will sit through.
 */
func TestASpokenTurnGetsTheOpeningWhole(t *testing.T) {
	root := t.TempDir()

	first := "I run a small studio in Sofia."
	second := strings.Repeat("More detail that will not fit. ", 40)

	if err := Write(root, first+"\n\n"+second); err != nil {
		t.Fatal(err)
	}

	block := Block(root, SpokenRunes)

	if !strings.Contains(block, first) {
		t.Errorf("the opening was lost: %q", block)
	}

	if strings.Contains(block, "More detail") {
		t.Errorf("the whole thing was sent to a spoken turn: %d letters", len([]rune(block)))
	}

	if strings.HasSuffix(strings.TrimSpace(block), "…") {
		t.Error("it was cut mid-sentence rather than at the paragraph")
	}
}

// A single paragraph too long to fit is cut, because there is nowhere better
// to stop — but it says so.
func TestOneLongParagraphIsCutAndSaysSo(t *testing.T) {
	root := t.TempDir()

	if err := Write(root, strings.Repeat("one long unbroken paragraph. ", 60)); err != nil {
		t.Fatal(err)
	}

	block := Block(root, SpokenRunes)

	if !strings.HasSuffix(strings.TrimSpace(block), "…") {
		t.Errorf("a cut paragraph does not say it was cut: %q", block)
	}
}

/*
 * It is labelled as facts, not dropped in unannounced.
 *
 * Prose put into a prompt with no label is read as something to respond to.
 * The first version of this produced an assistant that opened the conversation
 * by remarking on its owner's business.
 */
func TestTheProfileIsLabelledAsSomethingToWorkFromNotReplyTo(t *testing.T) {
	root := t.TempDir()

	if err := Write(root, "I run a small studio."); err != nil {
		t.Fatal(err)
	}

	block := Block(root, 0)

	if !strings.Contains(block, "not something to reply to") {
		t.Errorf("the profile is not labelled: %q", block)
	}

	if !strings.Contains(block, "I run a small studio.") {
		t.Errorf("the profile itself is missing: %q", block)
	}
}
