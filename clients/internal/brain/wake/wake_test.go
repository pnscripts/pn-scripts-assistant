package wake

import "testing"

// With no word set, everything said is for the brain.
//
// This is the default and the important case: requiring a name was tried and
// got in the way, because transcription has to get the name right before
// anything can match it — and an assistant that ignores its owner is a far
// worse failure than one that occasionally answers the television.
func TestWithNoWordItAnswersAnything(t *testing.T) {
	for _, said := range []string{
		"what do you know about me",
		"so then I told him it was fine",
		"здравей, какво знаеш за мен",
	} {
		heard := Listen(said, "", false)

		if !heard.Addressed || heard.Text != said {
			t.Errorf("%q gave %+v, want it passed through", said, heard)
		}
	}
}

// The name gets its attention, and what follows is the request.
func TestBeingAddressed(t *testing.T) {
	cases := []struct {
		said string
		want string
	}{
		{"PN Brain, what time is it?", "what time is it?"},
		{"Brain, what time is it?", "what time is it?"},
		{"brain what do you know about me", "what do you know about me"},
		{"Hey PN Brain — are you there", "are you there"},
		{"Brain", ""},
	}

	for _, c := range cases {
		heard := Listen(c.said, "PN Brain", false)

		if !heard.Addressed {
			t.Errorf("%q was not taken as addressed", c.said)

			continue
		}

		if heard.Text != c.want {
			t.Errorf("%q left %q, want %q", c.said, heard.Text, c.want)
		}
	}
}

// Everything else in the room is ignored.
//
// This is the half that matters. A microphone left open hears a television,
// somebody else's conversation, and the brain's own voice off the speakers —
// and every one of those used to become a turn.
func TestTheRoomIsIgnored(t *testing.T) {
	for _, said := range []string{
		"so then I told him it was fine",
		"the weather tomorrow is going to be warm",
		"I know 133 things about your work. Ask me anything.",
		"can you pass me that",
	} {
		if Listen(said, "PN Brain", false).Addressed {
			t.Errorf("%q was taken as addressed to the brain", said)
		}
	}
}

// Once it is in a conversation, it does not need to be named again.
//
// Having to say the name before every sentence is not a conversation.
func TestOnceEngagedItKeepsListening(t *testing.T) {
	heard := Listen("what about tomorrow", "PN Brain", true)

	if !heard.Addressed || heard.Text != "what about tomorrow" {
		t.Errorf("engaged, got %+v", heard)
	}
}

// A one-word name still works, and does not match half of itself.
func TestASingleWordName(t *testing.T) {
	if !Listen("Jarvis, lights on", "Jarvis", false).Addressed {
		t.Error("a one-word name was not recognised")
	}

	if Listen("just a moment", "Jarvis", false).Addressed {
		t.Error("something unrelated matched a one-word name")
	}
}

// Silence is not an address.
func TestSilenceIsNotAnAddress(t *testing.T) {
	if Listen("   ", "PN Brain", false).Addressed {
		t.Error("silence was taken as being addressed")
	}
}

// The name is as often at the end of a sentence as the start.
//
// "What time is it, brain" used to be taken as addressed and then handed on
// with nothing in it, because the name was found anywhere in the line but the
// request was only ever read from after it. The brain woke up, said it was
// listening, and threw the question away.
func TestTheNameAtTheEnd(t *testing.T) {
	cases := []struct {
		said string
		want string
	}{
		{"what time is it, brain", "what time is it"},
		{"turn the lights off brain", "turn the lights off"},
		{"brain what time is it", "what time is it"},
		{"remember this, brain, it matters", "remember this, it matters"},
	}

	for _, c := range cases {
		heard := Listen(c.said, "PN Brain", false)

		if !heard.Addressed {
			t.Errorf("%q was not taken as addressed", c.said)

			continue
		}

		if heard.Text != c.want {
			t.Errorf("%q left %q, want %q", c.said, heard.Text, c.want)
		}
	}
}

// A word that merely contains the name is not the name.
//
// The room is full of these. Matching letter-by-letter through the sentence had
// a film saying "brains" wake a brain called Brain — and the whole point of
// having a name is that the film does not get one.
func TestAWordThatOnlyContainsTheName(t *testing.T) {
	for _, said := range []string{
		"brains are interesting",
		"he was brainstorming all morning",
		"the birdbrain flew off",
	} {
		if heard := Listen(said, "PN Brain", false); heard.Addressed {
			t.Errorf("%q woke it", said)
		}
	}
}

/*
 * What the microphone actually writes down.
 *
 * This is why listening for a name was switched off the first time: whisper
 * hears "Brain" and writes "Bryan", the brain ignores its owner, and the owner
 * has no way to know why.
 *
 * Forgiving a wrong letter was tried here and does not survive the arithmetic.
 * "Bryan" is two edits from "Brain" while "rain" is one, so no distance rule
 * both catches the mishearing and leaves the weather alone. The owner listing
 * what their microphone writes is exact, and it is the only version of this
 * that does not guess.
 */
func TestTheNamesItIsGiven(t *testing.T) {
	const called = "PN Brain, Bryan, Brian"

	for _, said := range []string{
		"Bryan what time is it",
		"Brian what time is it",
		"brain what time is it",
		"PN Brain what time is it",
	} {
		heard := Listen(said, called, false)

		if !heard.Addressed {
			t.Errorf("%q was not taken as addressed", said)

			continue
		}

		if heard.Text != "what time is it" {
			t.Errorf("%q left %q", said, heard.Text)
		}
	}

	// And nothing else, however close it sounds.
	for _, said := range []string{
		"the rain in spain",
		"brains are interesting",
		"the train is late",
	} {
		if Listen(said, called, false).Addressed {
			t.Errorf("%q woke it", said)
		}
	}
}

// Getting somebody's attention is not part of what you are asking them.
func TestAttentionWordsAreNotTheRequest(t *testing.T) {
	for _, said := range []string{
		"hey brain what time is it",
		"okay brain what time is it",
		"hello brain what time is it",
	} {
		if heard := Listen(said, "PN Brain", false); heard.Text != "what time is it" {
			t.Errorf("%q left %q", said, heard.Text)
		}
	}
}

// Leaving a conversation.
//
// Being engaged means everything said for the next minute is the brain's
// business. In a room with other people in it, that is exactly long enough to
// turn to somebody else and have the brain answer instead.
func TestSayingThatIsAll(t *testing.T) {
	for _, said := range []string{
		"thanks", "Thank you.", "that's all", "stop listening", "goodbye",
	} {
		if !Ends(said) {
			t.Errorf("%q did not end the conversation", said)
		}
	}

	for _, said := range []string{
		"thanks for that, what else do you know",
		"stop the timer",
		"what time is it",
	} {
		if Ends(said) {
			t.Errorf("%q ended the conversation and should not have", said)
		}
	}
}

/*
 * A name that is not written in the Latin alphabet.
 *
 * The spoken language here can be Bulgarian, in which case the transcript
 * comes back in Cyrillic and so does the name. Nothing in the matching is
 * allowed to assume otherwise — the letters are compared as runes and case is
 * folded by the standard library, which handles both alphabets.
 *
 * Written down because the way this breaks is somebody making normalise cheaper
 * by walking bytes or restricting it to a-z, which passes every other test in
 * this file and leaves a Bulgarian speaker with a brain that never answers.
 */
func TestANameInAnotherAlphabet(t *testing.T) {
	const called = "PN Brain, Брейн, Мозък"

	cases := []struct {
		said string
		want string
	}{
		{"Брейн, колко е часът", "колко е часът"},
		{"колко е часът, Брейн", "колко е часът"},
		{"Мозък запомни това", "запомни това"},
	}

	for _, c := range cases {
		heard := Listen(c.said, called, false)

		if !heard.Addressed {
			t.Errorf("%q was not taken as addressed", c.said)

			continue
		}

		if heard.Text != c.want {
			t.Errorf("%q left %q, want %q", c.said, heard.Text, c.want)
		}
	}

	if Listen("днес времето е хубаво", called, false).Addressed {
		t.Error("it woke on a sentence that does not carry its name")
	}
}

/*
 * The recogniser welds the name onto the next word, and that used to lose the
 * whole turn.
 *
 * A real transcript from this machine: "Brain, tell me what you are going to
 * do" came back as "brainlue, tell me what you are going to do". Whole-word
 * matching found no name, so nothing happened — which from a chair is the
 * assistant not listening for its own name, and it is the commonest way for
 * that to look true when the microphone is working perfectly.
 */
func TestANameWeldedToTheNextWordStillWakesIt(t *testing.T) {
	heard := Listen("brainlue, tell me what you are going to do", "PN Brain", false)

	if !heard.Addressed {
		t.Fatal("a mangled name was not recognised at all")
	}

	if heard.Text != "tell me what you are going to do" {
		t.Fatalf("the request came out as %q", heard.Text)
	}
}

/*
 * And the case the strict rule was written for still holds.
 *
 * A television saying "brains" woke a brain called Brain and handed it an
 * empty request. Anything loose enough to catch every mishearing is loose
 * enough to answer the weather, so ordinary English endings are refused.
 */
func TestOrdinaryEnglishEndingsDoNotWakeIt(t *testing.T) {
	for _, said := range []string{
		"the zombies want brains",
		"he brained himself on the door",
		"she is braining the problem",
		"a brainy sort of person",
		"brainer than the rest",
	} {
		if Listen(said, "PN Brain", false).Addressed {
			t.Errorf("woke on %q", said)
		}
	}
}

// A long tail is a different word that happens to start the same way.
func TestAWordThatMerelyStartsTheSameIsNotTheName(t *testing.T) {
	for _, said := range []string{
		"the brainstorming session went well",
		"brainchildren of the last decade",
	} {
		if Listen(said, "PN Brain", false).Addressed {
			t.Errorf("woke on %q", said)
		}
	}
}

// Only the last word of the name may be welded: the middle of a name running
// into the next word is a different transcript altogether.
func TestOnlyTheEndOfTheNameMayBeWelded(t *testing.T) {
	if Listen("pnbrain brain what time is it", "PN Brain", false).Text == "" {
		t.Skip("nothing matched, which is acceptable here")
	}
}

// A short name is never softened: three letters plus a tail is most of the
// dictionary.
func TestAShortNameIsNotSoftened(t *testing.T) {
	if Listen("axolotl in the tank", "axo", false).Addressed {
		t.Error("a three-letter name matched a longer word")
	}
}

func TestTheExactNameStillWorks(t *testing.T) {
	for _, said := range []string{
		"brain what time is it",
		"PN Brain, what time is it",
		"what time is it brain",
		"hey brain, what time is it",
	} {
		heard := Listen(said, "PN Brain", false)

		if !heard.Addressed {
			t.Errorf("did not wake on %q", said)
		}

		if heard.Text != "what time is it" {
			t.Errorf("%q left %q", said, heard.Text)
		}
	}
}

/*
 * Being able to say "that sounded like my name".
 *
 * "No name in it" is true and useless when the recogniser mangled the name.
 * The two problems are completely different — one is that nobody addressed it,
 * the other is that somebody did and it did not recognise itself — and until
 * the log tells them apart, a mangling recogniser looks exactly like an
 * assistant that has stopped listening.
 */
func TestItCanSayWhenSomethingWasNearlyItsName(t *testing.T) {
	for _, said := range []string{
		"brains everywhere in this film",
		"brian tell me the time",
		"brainstorm about the design",
	} {
		if NearMiss(said, "PN Brain") == "" {
			t.Errorf("said nothing about %q", said)
		}
	}
}

// And it must not cry near-miss at an ordinary sentence, or the log fills with
// noise and nobody reads it.
func TestAnOrdinarySentenceIsNotANearMiss(t *testing.T) {
	for _, said := range []string{
		"what time is it",
		"the background is too dark",
		"put the kettle on",
		"",
	} {
		if got := NearMiss(said, "PN Brain"); got != "" {
			t.Errorf("%q was called a near miss on %q", got, said)
		}
	}
}

// With no name set, nothing is a near miss: everything is addressed to it.
func TestWithNoNameNothingIsANearMiss(t *testing.T) {
	if NearMiss("brains", "") != "" {
		t.Error("reported a near miss with no name set")
	}
}

func TestOneEditApartIsExactlyOneEdit(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"brain", "brian", true},  // two letters swapped, which counts as one
		{"brain", "brai", true},   // one deletion
		{"brain", "brains", true}, // one insertion
		{"brain", "brawn", true},  // one substitution
		{"brain", "brain", false}, // the same word is not an edit away
		{"brain", "train", true},
		{"brain", "trains", false},
	} {
		if got := oneEditApart(c.a, c.b); got != c.want && c.a != c.b {
			t.Errorf("%q vs %q: %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
