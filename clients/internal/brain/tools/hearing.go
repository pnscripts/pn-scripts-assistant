package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

/*
 * Asking the brain what it is hearing, and why it did or did not act on it.
 *
 * "It answered the television" and "it ignored me" are the two complaints, and
 * from a chair they are indistinguishable from each other and from a broken
 * microphone. Everything needed to tell them apart is measured — how loud the
 * room was, whose voice it was, whether this machine was playing something at
 * that moment — and until now all of it lived in a panel.
 *
 * A panel is the wrong place for it, because the question is asked out loud
 * and usually in the middle of the problem: somebody says "why did you say
 * that?" while music is playing, and being told to open a page is not an
 * answer. So it is a tool, in whatever language the question arrives in.
 */
type Hearing struct {
	// Recent hands back the last few things overheard, newest first.
	Recent func() []Overheard

	// State describes the machinery: whether a voice has been taught, whether
	// only that voice is answered, and whether this machine's own sound is
	// being cancelled out of the microphone.
	State func() HearingState
}

// Overheard is one thing the microphone made of the room.
type Overheard struct {
	Text      string
	Addressed bool
	Why       string

	KnownVoice bool
	Owner      bool
	Voice      float64

	MachinePlaying bool
	Playing        []string

	PeakRMS    int
	NoiseFloor int
	SpokeForMS int
	Ago        string
}

// HearingState is how the recognition is set up right now.
type HearingState struct {
	VoiceModel   bool
	VoiceTaught  bool
	OnlyOwner    bool
	Match        float64
	CancellingUs bool
	WakeWord     string
	AlwaysName   bool
}

func (Hearing) Name() string { return "what_am_i_hearing" }

func (Hearing) Description() string {
	return "Report what the microphone has picked up recently and what was decided about " +
		"each: whose voice it was, whether this machine was playing something at the " +
		"time, how loud the room was, and why each was answered or ignored. Use this for " +
		"\"why did you answer that\", \"why did you ignore me\", \"was that me\", \"is the " +
		"music confusing you\", or any question about what it can and cannot hear."
}

func (Hearing) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

// Safe: it reports what was already measured and changes nothing.
func (Hearing) Risk() Risk { return Safe }

func (Hearing) Summarize(json.RawMessage) string { return "Check what it has been hearing" }

func (t Hearing) Execute(context.Context, json.RawMessage) (string, error) {
	if t.State == nil || t.Recent == nil {
		return "", fmt.Errorf("nothing here is listening")
	}

	state := t.State()

	var b strings.Builder

	b.WriteString(describeHearing(state))

	recent := t.Recent()

	if len(recent) == 0 {
		b.WriteString("\n\nNothing has been overheard since I started.")

		return b.String(), nil
	}

	b.WriteString("\n\nThe last few things the microphone made of the room:\n")

	for i, h := range recent {
		if i >= 8 {
			break
		}

		fmt.Fprintf(&b, "\n%q — %s", strings.TrimSpace(h.Text), acted(h))

		if h.KnownVoice {
			whose := "not your voice"
			if h.Owner {
				whose = "your voice"
			}

			fmt.Fprintf(&b, "; %s (%.2f alike)", whose, h.Voice)
		}

		if h.MachinePlaying {
			what := "something on this machine"

			if len(h.Playing) > 0 {
				what = strings.Join(h.Playing, " and ")
			}

			fmt.Fprintf(&b, "; %s was playing at the time", what)
		}

		fmt.Fprintf(&b, "; peak %d against a room of %d", h.PeakRMS, h.NoiseFloor)
	}

	return b.String(), nil
}

func acted(h Overheard) string {
	if h.Addressed {
		return "answered"
	}

	if h.Why != "" {
		return "not answered: " + h.Why
	}

	return "not answered"
}

/*
 * describeHearing says how the listening is set up, in the order the questions
 * are actually asked.
 *
 * Each line is a thing that can be wrong on its own and is fixed differently,
 * so they are separate sentences rather than one summary that would be true
 * and useless.
 */
func describeHearing(s HearingState) string {
	var lines []string

	if s.AlwaysName && s.WakeWord != "" {
		lines = append(lines, fmt.Sprintf("I answer when I hear %q.", s.WakeWord))
	} else if s.WakeWord != "" {
		lines = append(lines, fmt.Sprintf("I answer to %q, and stay in the conversation "+
			"for a while afterwards.", s.WakeWord))
	} else {
		lines = append(lines, "I answer anything I hear; no name is needed.")
	}

	switch {
	case !s.VoiceModel:
		lines = append(lines, "I cannot tell one voice from another — what does that "+
			"is not installed. It can be fetched in Privacy.")

	case !s.VoiceTaught:
		lines = append(lines, "I can tell voices apart but have not been taught yours, "+
			"so I cannot tell you from anybody else. Teaching it takes three sentences, "+
			"in Privacy.")

	case s.OnlyOwner:
		lines = append(lines, fmt.Sprintf("I know your voice and answer only you; "+
			"anything below %.2f alike is heard and ignored.", s.Match))

	default:
		lines = append(lines, fmt.Sprintf("I know your voice and can tell it from others, "+
			"but I answer any voice — \"answer only me\" is switched off. "+
			"The line would be at %.2f.", s.Match))
	}

	if s.CancellingUs {
		lines = append(lines, "Anything this machine plays is subtracted from what I hear, "+
			"so music and videos on it do not reach me.")
	} else {
		lines = append(lines, "Music and videos playing on this machine reach the "+
			"microphone mixed with whoever is talking. Switching that off is one "+
			"setting — ask me to stop hearing this machine.")
	}

	return strings.Join(lines, " ")
}
