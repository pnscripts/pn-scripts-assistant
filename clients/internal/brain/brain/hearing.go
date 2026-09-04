package brain

import (
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
