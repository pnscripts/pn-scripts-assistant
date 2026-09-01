package brain

import (
	"strings"
	"testing"
)

/*
 * The name goes inside the sentence, before whatever ends it.
 *
 * Only the full stop was stripped, so a greeting ending in a question mark
 * became "Still up?, Petar." — punctuation in the middle of a question, in the
 * first words anybody hears, which makes every sentence after it sound
 * machine-made.
 *
 * Run many times because the name is added at random: once in two, so a single
 * call proves nothing either way.
 */
func TestTheGreetingIsNeverPunctuatedInTheMiddle(t *testing.T) {
	b := testBrain(t)

	for i := 0; i < 200; i++ {
		line := b.timeOfDay()

		for _, wrong := range []string{"?,", "!,", ".,"} {
			if strings.Contains(line, wrong) {
				t.Fatalf("greeting reads %q", line)
			}
		}

		if !strings.HasSuffix(line, ".") && !strings.HasSuffix(line, "?") &&
			!strings.HasSuffix(line, "!") {
			t.Fatalf("greeting does not end a sentence: %q", line)
		}
	}
}
