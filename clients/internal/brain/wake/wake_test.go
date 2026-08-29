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
