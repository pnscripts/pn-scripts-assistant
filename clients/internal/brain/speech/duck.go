package speech

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

/*
 * Turning the music down while the brain talks.
 *
 * The complaint is one sentence: when it is talking, it would be good to hear
 * it and not the background music. Which is what every phone and every car
 * does when a voice comes in over what is playing, and it is not the same
 * problem as the canceller — the canceller is about what the *microphone*
 * hears, and this is about what a person in the room hears.
 *
 * Both are needed and they fix opposite halves. With the canceller on, music
 * no longer confuses the recogniser but is still just as loud over the answer.
 * With this on, the answer is audible but the microphone still hears the
 * music. Neither substitutes for the other.
 *
 * Lowered rather than paused. Pausing is another program's business — it means
 * finding a media key to press and hoping the right window has focus — and
 * something that quietly pauses somebody's film every time it says a word is
 * worse than something that talks over it.
 */
const (
	// DuckedTo is what the rest is lowered to while it speaks.
	//
	// A fifth: quiet enough that a voice sits clearly on top, loud enough that
	// somebody can tell the music is still playing and has not been stopped.
	DuckedTo = 0.2

	/*
	 * HoldDucked is how long the level is held down after a sentence ends.
	 *
	 * An answer is spoken a sentence at a time, and each sentence is a
	 * separate go at the synthesiser, so restoring the moment one finishes
	 * makes the music surge back up between every sentence. That pumping is
	 * more annoying than not ducking at all.
	 */
	HoldDucked = 900 * time.Millisecond
)

/*
 * Remembered by application name rather than by node id.
 *
 * The id is the obvious key and it is wrong. A browser destroys and recreates
 * its stream whenever playback stops and starts, so the id we lowered may not
 * exist by the time we come to put it back — and WirePlumber persists a
 * stream's volume against the application, so the *next* stream it makes comes
 * up at the level we left. Restoring a stale id therefore fails silently and
 * leaves somebody's browser at a fifth for good, across restarts, with nothing
 * to suggest why.
 *
 * Observed exactly that way here: node 128 ducked, node 135 a minute later,
 * both at 0.2, neither restored.
 */
var (
	duckMu   sync.Mutex
	duckedAt map[string]float64
	speaking int
	restorer *time.Timer
)

/*
 * duckOthers lowers everything else playing and returns a way to let it back
 * up.
 *
 * Counted rather than a plain on and off, because sentences overlap: the next
 * one is being generated while the last is still being heard, and an
 * unbalanced pair would leave somebody's music at a fifth for the rest of the
 * evening.
 */
func duckOthers(ctx context.Context) func() {
	if !haveVolumeControl() {
		return func() {}
	}

	duckMu.Lock()

	speaking++

	if restorer != nil {
		restorer.Stop()
		restorer = nil
	}

	first := speaking == 1 && duckedAt == nil

	duckMu.Unlock()

	if first {
		lower(ctx)
	}

	return func() {
		duckMu.Lock()
		defer duckMu.Unlock()

		speaking--

		if speaking > 0 {
			return
		}

		speaking = 0

		// Held down for a moment, so the gap between two sentences of one
		// answer is not heard as the music jumping.
		restorer = time.AfterFunc(HoldDucked, restore)
	}
}

// lower takes down everything this machine is playing that is not us.
func lower(ctx context.Context) {
	found := map[string]float64{}

	for name, id := range otherStreams(ctx) {
		was, ok := volumeOf(ctx, id)
		if !ok {
			continue
		}

		// Already at or below where this would put it: left alone, and not
		// remembered, so it cannot be turned *up* when the answer ends.
		if was <= DuckedTo {
			continue
		}

		if setVolume(ctx, id, was*DuckedTo) {
			found[name] = was
		}
	}

	duckMu.Lock()

	if duckedAt == nil {
		duckedAt = found
	} else {
		for name, was := range found {
			duckedAt[name] = was
		}
	}

	note := duckedAt

	duckMu.Unlock()

	// And written down, because the failure that matters here survives the
	// program. See rememberDucked.
	rememberDucked(note)
}

/*
 * restore puts every level back where it was found.
 *
 * Its own context, not the turn's: by the time this runs the turn is over and
 * its context is cancelled, and leaving somebody's music at a fifth because
 * the answer was interrupted is exactly the kind of parting gift that gets a
 * program uninstalled.
 */
func restore() {
	duckMu.Lock()

	was := duckedAt
	duckedAt = nil
	restorer = nil

	duckMu.Unlock()

	if len(was) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	putBack(ctx, was)

	forgetDucked()
}

/*
 * putBack sets each remembered application back to the level it was found at.
 *
 * The ids are looked up again rather than remembered, because between lowering
 * and restoring a stream can have been destroyed and remade with a new one —
 * and setting a level on an id that no longer exists succeeds at nothing while
 * reporting nothing.
 */
func putBack(ctx context.Context, was map[string]float64) {
	now := otherStreams(ctx)

	for name, level := range was {
		id, playing := now[name]
		if !playing {
			continue
		}

		setVolume(ctx, id, level)
	}
}

// PutTheVolumeBack restores anything left turned down. Called when speech is
// stopped or the program is shutting down.
func PutTheVolumeBack() {
	duckMu.Lock()

	if restorer != nil {
		restorer.Stop()
		restorer = nil
	}

	speaking = 0

	duckMu.Unlock()

	restore()
}

// haveVolumeControl reports whether this machine has the tool for it. Without
// it everything here does nothing, quietly, which is right: a machine with no
// wireplumber still has to be able to talk.
func haveVolumeControl() bool {
	_, err := exec.LookPath("wpctl")

	return err == nil
}

/*
 * otherStreams lists what is playing that is not the brain itself.
 *
 * Its own voice is excluded for the obvious reason and its echo-cancel
 * playback for a less obvious one: that node carries the brain's voice into
 * the canceller, so turning it down would turn down the answer.
 */
func otherStreams(ctx context.Context) map[string]int {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(ctx, "pw-dump").Output()
	if err != nil {
		return nil
	}

	found, err := dumpObjects(raw)
	if err != nil {
		return nil
	}

	out := map[string]int{}

	for _, one := range found {
		var object struct {
			ID   int `json:"id"`
			Info struct {
				State string `json:"state"`
				Props struct {
					Class       string `json:"media.class"`
					Application string `json:"application.name"`
					Node        string `json:"node.name"`
				} `json:"props"`
			} `json:"info"`
		}

		if err := json.Unmarshal(one, &object); err != nil {
			continue
		}

		if object.Info.Props.Class != "Stream/Output/Audio" {
			continue
		}

		// Only what is actually making a noise. A paused video does not need
		// turning down, and turning it down would mean turning it up again
		// afterwards, which is somebody's film changing volume by itself.
		if object.Info.State != "running" {
			continue
		}

		name := object.Info.Props.Application
		if name == "" {
			name = object.Info.Props.Node
		}

		if ours(name) {
			continue
		}

		out[name] = object.ID
	}

	return out
}

// ours reports whether a stream is the brain's own voice.
func ours(name string) bool {
	lowered := strings.ToLower(name)

	for _, mine := range []string{"pn-brain", "pn_brain", "pw-play", "piper", "speech-dispatcher"} {
		if strings.Contains(lowered, mine) {
			return true
		}
	}

	return false
}

// volumeOf reads one stream's level, 1.0 being where it was left.
func volumeOf(ctx context.Context, id int) (float64, bool) {
	out, err := exec.CommandContext(ctx, "wpctl", "get-volume", strconv.Itoa(id)).Output()
	if err != nil {
		return 0, false
	}

	return readVolumeLine(string(out))
}

// readVolumeLine reads what wpctl prints: "Volume: 0.85", or on a muted stream
// "Volume: 0.85 [MUTED]".
func readVolumeLine(line string) (float64, bool) {
	fields := strings.Fields(line)

	/*
	 * The first word has to be the label.
	 *
	 * Without that check, "Node 51 not found" — which is what wpctl says about
	 * a stream that ended between being listed and being asked about — reads
	 * as a volume of 51. That is not a wrong number, it is a dangerous one:
	 * the level is restored by multiplying, so a stream would come back at
	 * five thousand per cent into somebody's speakers.
	 */
	if len(fields) < 2 || fields[0] != "Volume:" {
		return 0, false
	}

	level, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, false
	}

	// And a level outside what a volume can be is a misread, whatever it
	// says. PipeWire allows some headroom above unity, not five thousand.
	if level < 0 || level > 1.5 {
		return 0, false
	}

	return level, true
}

func setVolume(ctx context.Context, id int, to float64) bool {
	// Clamped at both ends. Everything here is arithmetic on a number read out
	// of another program's output, and the failure at the top end is somebody
	// being deafened.
	if to < 0 {
		to = 0
	}

	if to > 1.5 {
		to = 1.5
	}

	return exec.CommandContext(ctx, "wpctl", "set-volume", strconv.Itoa(id),
		strconv.FormatFloat(to, 'f', 3, 64)).Run() == nil
}

/*
 * Whether to turn the rest down at all.
 *
 * A package-level switch rather than a parameter threaded through every
 * caller: speaking happens from a dozen places — the greeting, a reminder, an
 * answer, a test — and every one of them would have to be told, which is a
 * dozen chances for one of them to be forgotten and for the setting to be true
 * except in the case somebody actually complained about.
 */
var ducking atomic.Bool

// DuckOthersWhileTalking turns the behaviour on or off. Off puts back
// immediately anything currently turned down.
func DuckOthersWhileTalking(on bool) {
	ducking.Store(on)

	if !on {
		PutTheVolumeBack()
	}
}

// DuckingOthers reports whether the rest of the room comes down while it talks.
func DuckingOthers() bool { return ducking.Load() }

func init() {
	// On unless somebody turns it off. Being talked over by a film is the
	// complaint that produced this, and a feature that has to be found before
	// it helps is a feature most people never get.
	ducking.Store(true)
}

// duckWhileTalking lowers the rest if that is switched on, and hands back the
// way to let it up again. A no-op when it is switched off, so callers do not
// each have to remember to ask.
func duckWhileTalking(ctx context.Context) func() {
	if !DuckingOthers() {
		return func() {}
	}

	return duckOthers(ctx)
}
