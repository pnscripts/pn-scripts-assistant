package speech

import (
	"strings"
	"testing"
)

// The memories this machine actually holds, abbreviated.
var realMemories = []string{
	"Petar regularly uses the website pnscripts.local (1160 visits in browser history).",
	"Petar uses pnscripts.local and staging.tools.swytchbike.com frequently.",
	"Petar regularly uses the website hosting.pnscripts.com (8 visits in browser history).",
	"Petar has a .pdf file named \"1-PnScripts-Skillo.pdf\" at /home/petar/Documents/PN Scripts/",
	"Petar's most-used websites: pnscripts.local (1160), staging.tools.swytchbike.com (240).",
	"Petar reads news on dnevnik.bg most mornings.",
}

/*
 * The names this person says are offered to the recogniser.
 *
 * Whisper knows the language and not the speaker: handed a name it has never
 * seen it returns the nearest thing in its vocabulary, confidently and with no
 * mark of doubt. "pnscripts.com" came back as "pncryptz.com" and a considered
 * opinion followed about a website that does not exist — a failure with
 * nothing in the sentence to give it away.
 */
func TestTheRecogniserIsToldTheNamesInUse(t *testing.T) {
	prompt := PromptFrom(realMemories)

	if prompt == "" {
		t.Fatal("nothing was offered to the recogniser at all")
	}

	// The one that was got wrong, and the one said most often.
	for _, want := range []string{"pnscripts.local", "hosting.pnscripts.com"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt does not mention %q:\n  %s", want, prompt)
		}
	}

	// Ordered by how often each is said, so the budget is spent on the names
	// most likely to come up rather than whichever sorted first.
	local := strings.Index(prompt, "pnscripts.local")
	rare := strings.Index(prompt, "dnevnik.bg")

	if local < 0 || (rare >= 0 && rare < local) {
		t.Errorf("a name said once outranks one said three times:\n  %s", prompt)
	}

	// Whisper truncates an over-long prompt silently, which would drop
	// whichever names happened to fall at the end.
	if words := len(strings.Fields(prompt)); words > MostPromptTokens {
		t.Errorf("the prompt is %d words, over the %d budget", words, MostPromptTokens)
	}
}

// Ordinary sentences contribute nothing, so the budget is not spent on words
// whisper already knows perfectly well.
func TestOrdinaryWordsAreNotTaughtBack(t *testing.T) {
	prompt := PromptFrom([]string{
		"Petar prefers tea to coffee and usually starts work at nine.",
		"Petar lives in Sofia and reads in the evenings.",
	})

	if prompt != "" {
		t.Errorf("ordinary sentences produced a vocabulary prompt: %q", prompt)
	}
}

/*
 * A name nothing here has met is flagged rather than acted on.
 *
 * This is the case no vocabulary can fix: the first time somebody says a
 * domain it is not in memory, in history, or in any file. Being nearly right
 * about a name is no use — a domain one letter out is somebody else's website
 * — so the honest move is to repeat it back, the way a person taking a name
 * over the telephone reads it out.
 */
func TestAnUnheardOfNameIsQueriedNotAssumed(t *testing.T) {
	SetVocabulary(func() []string { return realMemories })
	defer SetVocabulary(nil)

	// Known, in every dress it wears. Asking about these would be noise.
	for _, known := range []string{
		"what do you think of pnscripts.com?",
		"open hosting.pnscripts.com please",
		"check pnscripts.local",
	} {
		if got := Unfamiliar(known); len(got) > 0 {
			t.Errorf("asked about a name it knows well, from %q: %v", known, got)
		}
	}

	// Never seen. Worth one second of confirmation.
	got := Unfamiliar("have a look at quintarial-vex.io for me")

	if len(got) == 0 {
		t.Error("a name nothing here has ever seen passed without question")
	}

	// And ordinary speech is never queried, or the brain becomes unusable.
	if got := Unfamiliar("what is the weather in Sofia today?"); len(got) > 0 {
		t.Errorf("an ordinary sentence raised a query: %v", got)
	}
}
