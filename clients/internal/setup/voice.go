package setup

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/speech"
)

/*
 * Hearing the voice before agreeing to it.
 *
 * Setup used to install a voice and never let anybody hear it, so the first
 * sample of what the assistant sounds like arrived after setup had closed —
 * at which point changing it means finding a settings panel and knowing that
 * "en_GB-alba-medium" is a woman. The question people have is "what will it
 * sound like", and the only honest answer to it is the sound.
 *
 * Three, because that is the whole choice: a robot, a man, a woman. The names
 * of the models behind them are not offered, for the same reason a car is not
 * sold by the part number of its engine.
 */

// errNoVoice is the one failure worth its own value: it is not an error in the
// speaking, it is the voice not being installed, and the page says so rather
// than reporting that something went wrong.
var errNoVoice = errors.New("that voice is not installed yet")

type voiceView struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Detail string `json:"detail"`

	// Available is false when the voice needs a download that has not
	// happened. Shown greyed rather than hidden: "there is a woman's voice and
	// it is not installed yet" is a different message from "there is no
	// woman's voice", and hiding the row sends the second one.
	Available bool `json:"available"`
}

/*
 * voiceChoices reduces every installed voice to the three that get offered.
 *
 * speech.Voices lists whatever is on the machine, which on a full install is
 * a robot, an older robot and two humans, and on a bare one is a single espeak
 * robot. Picking one of each kind here keeps the setup question the same shape
 * on both machines.
 */
func voiceChoices() []voiceView {
	installed := speech.Voices()

	find := func(sex string) (speech.Voice, bool) {
		// A neural voice in preference to the espeak fallback: both are
		// robots, only one of them is the one this will actually use.
		for _, v := range installed {
			if v.Sex == sex && v.Engine == "piper" {
				return v, true
			}
		}

		for _, v := range installed {
			if v.Sex == sex {
				return v, true
			}
		}

		return speech.Voice{}, false
	}

	out := make([]voiceView, 0, 3)

	for _, want := range []struct{ kind, name, detail string }{
		{"robot", "A robot", "a machine, and clear about it"},
		{"man", "A man", "a British man's voice"},
		{"woman", "A woman", "a British woman's voice"},
	} {
		view := voiceView{Kind: want.kind, Name: want.name, Detail: want.detail}

		if v, ok := find(want.kind); ok {
			view.ID = v.ID
			view.Available = true
		} else {
			view.Detail = "arrives with Voice (speaking)"
		}

		out = append(out, view)
	}

	return out
}

/*
 * sampleLine is what each voice says, and it is the same line every time.
 *
 * Comparing two voices means hearing the same words in both. Different
 * sentences would make the choice partly about the sentence.
 */
func (s *Server) sampleLine() string {
	name := config.DefaultName

	// Whatever it has been named, if setup has already been told. The sample
	// is a rehearsal of a real answer, and a real answer says its own name.
	if data, err := os.ReadFile(s.envPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if rest, found := strings.CutPrefix(line, "BRAIN_NAME="); found {
				if v := strings.TrimSpace(rest); v != "" {
					name = v
				}
			}
		}
	}

	return "Hello. I am " + name + ", and this is how I will sound when I answer you."
}

/*
 * hear speaks one sample and waits for it to finish.
 *
 * Waiting matters: the page disables its buttons while this runs, and a
 * request that returned the moment the words were queued would re-enable them
 * over the top of a voice that was still talking — so two clicks would make
 * two voices speak at once.
 */
func (s *Server) hear(kind string) error {
	var chosen voiceView

	for _, v := range voiceChoices() {
		if v.Kind == kind {
			chosen = v
		}
	}

	if !chosen.Available {
		return errNoVoice
	}

	speech.SetVoice(chosen.ID)

	// Long enough for a slow processor to synthesise one sentence, short
	// enough that a wedged engine does not hold the page forever.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	return speech.SpeakAndWait(ctx, s.sampleLine())
}

// chooseVoice records which voice the assistant will answer in.
func (s *Server) chooseVoice(kind string) error {
	var chosen voiceView

	for _, v := range voiceChoices() {
		if v.Kind == kind {
			chosen = v
		}
	}

	if !chosen.Available {
		return errNoVoice
	}

	s.mu.Lock()
	s.voice = kind
	s.mu.Unlock()

	speech.SetVoice(chosen.ID)

	return s.writeSetting("BRAIN_VOICE", chosen.ID, 0o600)
}
