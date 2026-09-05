package brain

import (
	"context"
	"fmt"
	"os"
	"pn-brain/internal/brain/places"
	"pn-brain/internal/brain/storage"
	"strconv"
	"strings"
	"time"

	"pn-brain/internal/brain/llm"
	"pn-brain/internal/brain/pace"
	"pn-brain/internal/brain/speech"
	"pn-brain/internal/brain/tools"
	"pn-brain/internal/brain/voiceprint"
	"pn-brain/internal/preflight"
)

/*
 * What the brain can say about its own listening.
 *
 * The record of what was overheard is kept by the part that does the
 * listening, which is the server — it is the only thing that sees a turn
 * arrive and decides what to do with it. The tool that answers "why did you
 * ignore me" lives here, with the other tools.
 *
 * So the two are joined by a function the listener fills in. Not by moving the
 * record here, which would put the log of what the microphone heard in the
 * same package as the model and the memory; and not by handing the tool the
 * server, which would let a tool reach the whole of it.
 */

// Listening is filled in by whatever is doing the listening, and is nil until
// it is.
var Listening func() []tools.Overheard

// recentlyHeard is the last few things the microphone made of the room.
func (b *Brain) recentlyHeard() []tools.Overheard {
	if Listening == nil {
		return nil
	}

	return Listening()
}

// howItListens describes the machinery, so the tool can say which of the
// several separate things is switched on.
func (b *Brain) howItListens() tools.HearingState {
	return tools.HearingState{
		VoiceModel:   voiceprint.Installed(),
		VoiceTaught:  voiceprint.Enrolled(b.Root),
		OnlyOwner:    b.Cfg.OnlyMe,
		Match:        b.Cfg.VoiceMatch,
		CancellingUs: speech.Rerouting(),
		WakeWord:     strings.TrimSpace(b.Cfg.WakeWord),
		AlwaysName:   b.Cfg.AlwaysName,
	}
}

/*
 * howFast is the measured pace of the last few turns, and what it means here.
 *
 * The arithmetic lives in the pace package and the machine facts in preflight;
 * this joins them, because the same numbers mean different things depending on
 * what they were measured on.
 */
func (b *Brain) howFast() tools.Speed {
	typical, over := pace.Typical()

	roles := b.modelRoles()
	hardware := preflight.DetectHardware()

	s := tools.Speed{
		Over:     over,
		Verdict:  pace.Verdict(typical, over),
		Hearing:  typical.Hearing,
		Thinking: typical.Thinking,
		Writing:  typical.Writing,
		Speaking: typical.Speaking,
		ToFirst:  typical.ToFirst,
		Model:    typical.Model,
		Quick:    roles.Talk,
	}

	if s.Model == "" {
		s.Model = roles.Work
	}

	for _, f := range pace.Findings(typical, over, pace.Machine{
		Cores:    hardware.CPUCores,
		MemoryGB: hardware.RAMGB,
		HasGPU:   hardware.HasGPU,
		GPUName:  hardware.GPUName,
		Working:  roles.Work,
		Quick:    roles.Talk,
		Suggest:  llm.TalkCandidates[0],
	}) {
		s.Findings = append(s.Findings, tools.Finding{
			Stage: f.Stage, Costs: f.Costs, Because: f.Because, Change: f.Change,
		})
	}

	return s
}

// Ago is how long ago something was heard, in the words somebody would use.
func Ago(at time.Time) string {
	since := time.Since(at)

	switch {
	case since < time.Minute:
		return "just now"
	case since < time.Hour:
		return strconv.Itoa(int(since.Minutes())) + " minutes ago"
	}

	return strconv.Itoa(int(since.Hours())) + " hours ago"
}

/*
 * somewhereToStart is the drives and folders the brain can see for itself.
 *
 * The home directory and whatever is mounted, named the way a person would
 * name them. A closed list assembled from the machine — which is what makes it
 * safe to act on a spoken instruction: "learn everything" can pick from this
 * without any chance that a misheard word becomes a folder somebody never
 * meant it to read.
 */
func (b *Brain) somewhereToStart() []tools.Candidate {
	drives, err := storage.Drives(b.Root)
	if err != nil {
		return nil
	}

	var out []tools.Candidate

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, tools.Candidate{
			Path: home, Name: "your home folder", Home: true,
		})
	}

	for _, d := range drives {
		if d.MountPoint == "" || d.MountPoint == "/" {
			continue
		}

		// The home directory is already offered above, under a name somebody
		// recognises.
		if strings.HasPrefix(d.MountPoint, "/home/") {
			continue
		}

		out = append(out, tools.Candidate{
			Path: d.MountPoint,
			Name: places.Short(d.MountPoint),
		})
	}

	return out
}

/*
 * The settings somebody can change by saying so.
 *
 * Deliberately not all of them. Privacy is set from setup and the privacy
 * panel and nowhere else, for the same reason the protected-paths list is: a
 * conversation that can talk the brain into sharing more is a conversation
 * somebody else can have with it.
 */
func (b *Brain) settingsNow() tools.Setting {
	return tools.Setting{
		SpeakAloud: b.Cfg.AlwaysSpeak,
		NameEvery:  b.Cfg.AlwaysName,
		OnlyOwner:  b.Cfg.OnlyMe,
		KeepQuiet:  b.Cfg.KeepQuiet,
		Cancelling: speech.Rerouting(),
		Voice:      speech.VoiceKind(b.Cfg.Voice),
	}
}

// changeSetting applies one switch and says what it now means.
func (b *Brain) changeSetting(ctx context.Context, what string, on bool) (string, error) {
	switch what {
	case "speak_aloud":
		b.Cfg.AlwaysSpeak = on

		if on {
			return "I will read my answers aloud.", b.Cfg.Save(b.Root)
		}

		return "I will answer in writing and stay quiet.", b.Cfg.Save(b.Root)

	case "name_every":
		b.Cfg.AlwaysName = on

		if on {
			return fmt.Sprintf("I will only answer when I hear %q.", b.Cfg.WakeWord),
				b.Cfg.Save(b.Root)
		}

		return "You need my name to start, and then I stay in the conversation " +
			"for a while without it.", b.Cfg.Save(b.Root)

	case "only_me":
		b.Cfg.OnlyMe = on

		if on && !voiceprint.Enrolled(b.Root) {
			b.Cfg.OnlyMe = false

			return "I have not been taught your voice yet, so answering only you " +
				"would mean answering nobody. Teach me in Privacy — three " +
				"sentences — and ask me again.", nil
		}

		if on {
			return "I will answer your voice and ignore the rest of the room.",
				b.Cfg.Save(b.Root)
		}

		return "I will answer any voice that addresses me.", b.Cfg.Save(b.Root)

	case "keep_quiet":
		b.Cfg.KeepQuiet = on

		speech.DuckOthersWhileTalking(on)

		if on {
			return "Music and videos will drop while I speak and come back after.",
				b.Cfg.Save(b.Root)
		}

		return "I will speak over whatever is playing.", b.Cfg.Save(b.Root)

	case "cancel_room":
		if !on {
			speech.PutTheSoundBack(ctx)

			b.Cfg.CancelRoom = false

			return "This machine's sound goes straight to the speakers again, so I " +
					"hear it through the microphone along with everything else.",
				b.Cfg.Save(b.Root)
		}

		if err := speech.CancelWhatThisMachinePlays(ctx); err != nil {
			return "", err
		}

		b.Cfg.CancelRoom = true

		return "Everything this machine plays now goes through the canceller, so I " +
				"no longer hear it. Sound from somewhere else in the room still reaches me.",
			b.Cfg.Save(b.Root)
	}

	return "", fmt.Errorf("there is no setting called %q", what)
}

/*
 * changeVoice picks the robot, a man's voice or a woman's.
 *
 * The three are kept as three because they were asked for as three: a clearer
 * voice was wanted without losing the choice.
 */
func (b *Brain) changeVoice(ctx context.Context, which string) (string, error) {
	id := speech.PickVoice(which, b.Cfg.Language)

	if id == "" {
		return fmt.Sprintf("There is no %s voice installed. The ones that are "+
			"installed are listed under Engine.", which), nil
	}

	speech.SetVoice(id)

	b.Cfg.Voice = id

	if err := b.Cfg.Save(b.Root); err != nil {
		return "", err
	}

	return fmt.Sprintf("This is the %s voice.", which), nil
}
