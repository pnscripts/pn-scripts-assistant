package speech

import (
	"strings"
	"testing"
)

/*
 * An answer that is only code is not read aloud.
 *
 * It is on the screen, in a box designed for reading, and saying it is worse
 * than silence: thirty lines of Go read out is a minute of noise ending in
 * somebody switching the voice off for good.
 */
func TestAnAnswerThatIsOnlyCodeIsNotSpoken(t *testing.T) {
	if _, worth := WorthSaying("```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```"); worth {
		t.Error("a block of code was read aloud")
	}

	if _, worth := WorthSaying("| name | size |\n|---|---|\n| a.go | 12 |"); worth {
		t.Error("a table was read aloud")
	}

	if _, worth := WorthSaying(""); worth {
		t.Error("nothing was read aloud")
	}
}

// But the sentence around the code is, without the code in it.
func TestTheSentenceAroundTheCodeIsSpoken(t *testing.T) {
	spoken, worth := WorthSaying(
		"I found the bug in the loop.\n\n```go\nfor i := range n {\n}\n```\n\nIt is fixed now.")

	if !worth {
		t.Fatal("an answer with a sentence in it was not spoken")
	}

	if strings.Contains(spoken, "for i") || strings.Contains(spoken, "range") {
		t.Errorf("the code was read aloud: %q", spoken)
	}

	for _, want := range []string{"found the bug", "fixed now"} {
		if !strings.Contains(spoken, want) {
			t.Errorf("%q is missing from %q", want, spoken)
		}
	}
}

/*
 * The marks a written answer is laid out with mean nothing said aloud.
 *
 * Headings, bullets and emphasis are how something is read, not what it says.
 * A voice that reads them is spelling out punctuation.
 */
func TestLayoutIsNotReadOut(t *testing.T) {
	spoken, _ := WorthSaying("## What I found\n\n- **Three** files changed\n- One test `failed`")

	for _, unwanted := range []string{"#", "*", "`", "-"} {
		if strings.Contains(spoken, unwanted) {
			t.Errorf("%q survived into %q", unwanted, spoken)
		}
	}

	if !strings.Contains(spoken, "Three files changed") {
		t.Errorf("the words were lost: %q", spoken)
	}
}

// A path on a line of its own is a spelling test when spoken and the answer
// when looked at, so it stays on the screen.
func TestAPathOnItsOwnLineIsNotSpelledOut(t *testing.T) {
	spoken, worth := WorthSaying(
		"It lives here:\n/media/petar/c8fc2986-4b79/DEV/Projects/pnscripts/products\nHave a look.")

	if !worth {
		t.Fatal("nothing was spoken")
	}

	if strings.Contains(spoken, "c8fc2986") {
		t.Errorf("the path was read aloud: %q", spoken)
	}
}

/*
 * A short answer is spoken whole, and a long one is cut at a sentence.
 *
 * Whole, because cutting a paragraph is the annoying case: somebody hears most
 * of an answer and has to go to the screen for the last line. And at a
 * sentence, because a voice that stops mid-clause sounds like it has crashed.
 */
func TestShortIsWholeAndLongStopsAtASentence(t *testing.T) {
	short := "Yes, it builds. Four tests, all passing."

	spoken, _ := WorthSaying(short)

	if spoken != short {
		t.Errorf("a short answer came back as %q", spoken)
	}

	long := strings.Repeat("This is one more sentence about the thing. ", 60)

	spoken, worth := WorthSaying(long)

	if !worth {
		t.Fatal("a long answer was not spoken at all")
	}

	if len([]rune(spoken)) > MostSpoken {
		t.Errorf("%d runes were read aloud", len([]rune(spoken)))
	}

	if !strings.HasSuffix(strings.TrimSpace(spoken), ".") {
		t.Errorf("it stopped mid-sentence: %q", spoken)
	}
}

/*
 * The limit is in runes, not bytes.
 *
 * Half the sentences this program produces are in Bulgarian, where every
 * letter is two bytes — a limit counted in bytes would cut a Bulgarian answer
 * in half and an English one not at all.
 */
func TestTheLimitIsInLettersNotBytes(t *testing.T) {
	bulgarian := strings.Repeat("Това е още едно изречение за нещото. ", 60)

	spoken, worth := WorthSaying(bulgarian)

	if !worth {
		t.Fatal("a Bulgarian answer was not spoken")
	}

	runes := len([]rune(spoken))

	if runes > MostSpoken {
		t.Errorf("%d letters were read aloud", runes)
	}

	// And it is not half of what an English answer would get, which is what a
	// byte limit would have produced.
	if runes < MostSpoken/2 {
		t.Errorf("only %d letters of a Bulgarian answer were read aloud", runes)
	}
}
