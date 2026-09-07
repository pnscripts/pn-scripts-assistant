package brain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
 * somewhereToStart is the folders the brain can see and could actually take on.
 *
 * Could actually take on is the whole of it. The first version offered the
 * home folder and the drive the brain lives on, which are the two obvious
 * answers and are both refused: a whole home folder is more than anybody means
 * by "learn everything", and a brain cannot learn from the drive it keeps
 * itself on. So "learn everything" was approved and then did nothing, with two
 * refusals where the work should have been — which is worse than saying no,
 * because somebody had already agreed to it.
 *
 * It offers what is inside them instead. A closed list still, assembled by
 * looking at the machine rather than parsed out of speech, which is what makes
 * it safe to act on a spoken instruction.
 */
func (b *Brain) somewhereToStart() []tools.Candidate {
	var out []tools.Candidate

	seen := map[string]bool{}

	add := func(path, name string, home bool) {
		clean := filepath.Clean(path)

		if seen[clean] {
			return
		}

		// Only somewhere that is there, is a folder, and is not refused for a
		// reason the person would then have to be told about.
		if info, err := os.Stat(clean); err != nil || !info.IsDir() {
			return
		}

		if err := places.CanWatch(b.Root, clean); err != nil {
			return
		}

		seen[clean] = true

		out = append(out, tools.Candidate{Path: clean, Name: name, Home: home})
	}

	/*
	 * The folders inside home that people keep work in.
	 *
	 * Named rather than discovered, because a home folder also holds fifty
	 * dotfiles and a Trash, and offering somebody their own .cache is not an
	 * offer anybody wants.
	 */
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, kind := range []struct{ dir, name string }{
			{"Documents", "your documents"},
			{"Desktop", "your desktop"},
			{"Downloads", "your downloads"},
			{"Projects", "your projects"},
			{"Development", "your development folder"},
			{"DEV", "your development folder"},
			{"Work", "your work folder"},
		} {
			add(filepath.Join(home, kind.dir), kind.name, true)
		}
	}

	/*
	 * And what is on the drives, one level in.
	 *
	 * The root of the drive the brain lives on is refused — it cannot learn
	 * from where it keeps itself — so the folders beside the brain are what is
	 * actually available, and on this machine that is where the work is.
	 */
	drives, err := storage.Drives(b.Root)
	if err != nil {
		return out
	}

	for _, d := range drives {
		if d.MountPoint == "" || d.MountPoint == "/" ||
			strings.HasPrefix(d.MountPoint, "/home/") {
			continue
		}

		/*
		 * A name somebody would use for the drive, not its mount point.
		 *
		 * "DEV on /media/petar/c8fc2986-4b79-4d7b-9a8c-e6db653915ac" is a
		 * path with a folder glued to the front of it. What somebody calls
		 * that is the external drive.
		 */
		where := "the external drive"

		if !d.Removable {
			where = filepath.Base(d.MountPoint)
		}

		// The drive itself, when it is not the one the brain is on.
		add(d.MountPoint, where, false)

		entries, err := os.ReadDir(d.MountPoint)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || housekeeping(e.Name()) {
				continue
			}

			at := filepath.Join(d.MountPoint, e.Name())

			// Never its own storage, whatever the folder is called. See
			// itsOwnData — offering it is offering to read its own memories
			// back in as somebody else's documents.
			if itsOwnData(at) {
				continue
			}

			add(at, e.Name()+" on "+where, false)
		}
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
		Loudness:   speech.Loudness(),
	}
}

/*
 * changeLoudness moves its own voice up or down a step.
 *
 * Steps rather than a number, because "louder" is what somebody says and
 * "zero point six" is what a settings file says. Bounded at both ends: below a
 * fifth it can be heard talking and not understood, which is worse than
 * silence, and above full it would be distortion rather than volume.
 */
func (b *Brain) changeLoudness(_ context.Context, step string) (string, error) {
	const move = 0.15

	now := speech.Loudness()

	switch step {
	case "louder":
		now += move
	case "quieter":
		now -= move
	case "full":
		now = speech.FullLevel
	case "lowest":
		now = speech.QuietestUseful
	default:
		return "", fmt.Errorf("say louder, quieter, full or lowest")
	}

	speech.SetLoudness(now)

	b.Cfg.VoiceLoudness = speech.Loudness()

	if err := b.Cfg.Save(b.Root); err != nil {
		return "", err
	}

	at := int(speech.Loudness() * 100)

	switch {
	case at >= 100:
		return "This is as loud as I go — and this is my voice only, nothing else " +
			"on the machine.", nil

	case speech.Loudness() <= speech.QuietestUseful:
		return "This is as quiet as I go. Any lower and you would hear me talking " +
			"without making out the words.", nil
	}

	return fmt.Sprintf("My voice is at %d%% now. Anything else playing is untouched.",
		at), nil
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

/*
 * housekeeping names folders that belong to the filesystem rather than to
 * anybody.
 *
 * Offering somebody their own lost+found is not an offer. Every one of these
 * exists on drives people actually use, and each would be read, embedded and
 * remembered at the cost of the folders that matter.
 */
func housekeeping(name string) bool {
	switch name {
	case "lost+found", "System Volume Information", "$RECYCLE.BIN",
		".Trash-1000", "snap", "PN-BRAIN-DATA":
		return true
	}

	return false
}

/*
 * itsOwnData reports whether a folder is the brain's own storage.
 *
 * By what is in it rather than by what it is called. PN-BRAIN-DATA was on the
 * list above and a backup of it beside it was not, so the brain offered to
 * learn from a folder holding a JSON dump of everything it had ever known —
 * which is a loop: it would read its own memories back in as documents,
 * attributed to a file, and propose remembering them again.
 *
 * A name list cannot catch this. Somebody's backup is called whatever they
 * called it, and the copies feature makes them on purpose.
 */
func itsOwnData(path string) bool {
	for _, mark := range []string{"brain.sqlite", "brain.conf", ".brain-root.json"} {
		if _, err := os.Stat(filepath.Join(path, mark)); err == nil {
			return true
		}
	}

	return false
}

// SomewhereToStart is what it would offer, for the interface and for tests.
func (b *Brain) SomewhereToStart() []tools.Candidate { return b.somewhereToStart() }
