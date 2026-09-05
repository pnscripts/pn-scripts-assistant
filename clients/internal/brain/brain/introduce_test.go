package brain

import (
	"strings"
	"testing"
)

/*
 * Saying what it can do, once.
 *
 * Everything this program can be asked for is reachable by saying a sentence,
 * and none of it is discoverable by saying a sentence — its owner was finding
 * out what to say by asking the person who wrote it.
 */
func TestOnFirstMeetingItSaysToAskWhatItCanDo(t *testing.T) {
	root := t.TempDir()

	line := introductionLine(root, []string{"read_file", "change_a_setting"})

	if !strings.Contains(line, "ask me what I can do") {
		t.Fatalf("said nothing useful on first meeting: %q", line)
	}
}

/*
 * And then never again, which is the harder half.
 *
 * The greeting recited every launch is the fault this must not repeat. An
 * introduction that keeps introducing teaches somebody to stop reading the
 * first line, which is where everything that matters goes.
 */
func TestItDoesNotIntroduceItselfTwice(t *testing.T) {
	root := t.TempDir()

	loaded := []string{"read_file", "change_a_setting"}

	if introductionLine(root, loaded) == "" {
		t.Fatal("said nothing at all the first time")
	}

	if err := MarkIntroduced(root, loaded); err != nil {
		t.Fatal(err)
	}

	if again := introductionLine(root, loaded); again != "" {
		t.Fatalf("introduced itself a second time: %q", again)
	}
}

/*
 * Something new is worth one sentence.
 *
 * A tool added between one launch and the next is the only thing that can
 * make the program able to do something its owner has no way of knowing about.
 */
func TestSomethingNewIsMentionedOnce(t *testing.T) {
	root := t.TempDir()

	MarkIntroduced(root, []string{"read_file"})

	line := introductionLine(root, []string{"read_file", "change_a_setting"})

	if line == "" {
		t.Fatal("said nothing about a tool it did not have last time")
	}

	if !strings.Contains(line, "answer only me") {
		t.Fatalf("did not say what to actually say: %q", line)
	}

	MarkIntroduced(root, []string{"read_file", "change_a_setting"})

	if again := introductionLine(root, []string{"read_file", "change_a_setting"}); again != "" {
		t.Fatalf("mentioned the same new tool twice: %q", again)
	}
}

// A tool name is written for a model. "Say places_it_learns_from" is not an
// instruction anybody can follow.
func TestItSuggestsWordsAPersonWouldSay(t *testing.T) {
	for _, c := range []struct {
		tool string
		want string
	}{
		{"places_it_learns_from", "learn everything"},
		{"what_am_i_hearing", "why did you ignore me"},
		{"how_fast_can_you_answer", "why are you so slow"},
	} {
		if got := firstExample([]string{c.tool}); !strings.Contains(got, c.want) {
			t.Errorf("%s suggested %q, want something like %q", c.tool, got, c.want)
		}
	}

	// And something with no phrase of its own still gets a usable one.
	if got := firstExample([]string{"some_new_tool"}); !strings.Contains(got, "what can you do") {
		t.Errorf("fell back to %q", got)
	}
}

// A record that cannot be read is not a reason to stay silent forever.
func TestAnUnreadableRecordMeansItIntroducesItself(t *testing.T) {
	root := t.TempDir()

	if EverIntroduced(root) {
		t.Fatal("claimed to have introduced itself with no record at all")
	}
}
