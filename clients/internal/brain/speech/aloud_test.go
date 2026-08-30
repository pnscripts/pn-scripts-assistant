package speech

import "testing"

/*
 * Sentences are cut where a person would pause, not at every full stop.
 *
 * Speaking a fragment is worse than waiting for the rest of it: the synthesiser
 * puts the wrong tune on half a clause and the pause lands in the middle of the
 * thought. And a full stop inside a number or an initial is not an ending at
 * all — cutting there starts the voice off mid-number.
 */
func TestCuttingSentences(t *testing.T) {
	cases := []struct {
		text     string
		sentence string
		rest     string
		found    bool
	}{
		{"Good morning, Petar. How can I help?", "Good morning, Petar.", "How can I help?", true},
		{"One moment", "", "One moment", false},
		{"Yes! That worked.", "Yes!", "That worked.", true},
		{"Is it ready? I think so.", "Is it ready?", "I think so.", true},
		{"A line\nand another", "A line", "and another", true},

		// Not endings.
		{"It costs 3.50 in total", "", "It costs 3.50 in total", false},
		{"Ask J. Smith about it", "", "Ask J. Smith about it", false},
		{"Waiting for the rest of this sentence", "", "Waiting for the rest of this sentence", false},
	}

	for _, c := range cases {
		sentence, rest, found := cutSentence(c.text)

		if found != c.found {
			t.Errorf("%q: found=%v, want %v (got %q)", c.text, found, c.found, sentence)

			continue
		}

		if !found {
			continue
		}

		if sentence != c.sentence {
			t.Errorf("%q gave sentence %q, want %q", c.text, sentence, c.sentence)
		}

		if rest != c.rest {
			t.Errorf("%q left %q, want %q", c.text, rest, c.rest)
		}
	}
}

/*
 * Text arrives a few characters at a time, and whole sentences come out.
 *
 * This is the shape the model produces: not lines, not sentences, but three or
 * four characters at a time as they are computed.
 */
func TestPiecesBecomeSentences(t *testing.T) {
	const answer = "Good morning. I have four things for you. Shall I start?"

	var out []string

	// The same gathering the speaker does, without a voice attached.
	var pending string

	for i := 0; i < len(answer); i += 3 {
		end := i + 3
		if end > len(answer) {
			end = len(answer)
		}

		pending += answer[i:end]

		for {
			sentence, rest, found := cutSentence(pending)
			if !found {
				break
			}

			pending = rest

			out = append(out, sentence)
		}
	}

	if pending != "" {
		out = append(out, pending)
	}

	want := []string{"Good morning.", "I have four things for you.", "Shall I start?"}

	if len(out) != len(want) {
		t.Fatalf("got %d sentences, want %d: %q", len(out), len(want), out)
	}

	for i := range want {
		if out[i] != want[i] {
			t.Errorf("sentence %d is %q, want %q", i, out[i], want[i])
		}
	}
}
