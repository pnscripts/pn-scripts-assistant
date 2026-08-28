package wake

import "testing"

// The name gets its attention, and what follows is the request.
func TestBeingAddressed(t *testing.T) {
	cases := []struct {
		said string
		want string
	}{
		{"PN Brain, what time is it?", "what time is it?"},
		{"Brain, what time is it?", "what time is it?"},
		{"brain what do you know about me", "what do you know about me"},
		{"Hey PN Brain — are you there", "— are you there"},
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
