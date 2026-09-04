package brain

import (
	"strconv"
	"strings"
	"time"

	"pn-brain/internal/brain/speech"
	"pn-brain/internal/brain/tools"
	"pn-brain/internal/brain/voiceprint"
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
