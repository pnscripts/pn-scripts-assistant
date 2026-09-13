package speech

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
 * Lowered rather than paused. Pausing is another program's business, and
 * something that quietly pauses somebody's film every time it says a word is
 * worse than something that talks over it.
 *
 * How it is lowered is the whole of this file, and it was wrong for a long
 * time. It used to set each program's volume with wpctl and set it back
 * afterwards. That volume is not the stream's to lose: WirePlumber saves it
 * against the program, and the next stream that program opens — tomorrow,
 * after a reboot — comes up at whatever was saved. So every way of not getting
 * to "afterwards" left a program quiet for good:
 *
 *   - the stream ended before the level went back, which is every sentence of
 *     the brain's own voice and every browser that stopped a video;
 *   - the brain turned down its own voice, because espeak's stream is called
 *     "eSpeak" and that was not on the list of names it knew as its own — and
 *     from then on every answer was spoken at a hundredth of the level;
 *   - the note meant to finish the job next time was overwritten by the next
 *     sentence, or shared by two brains on one machine;
 *   - "a fifth" was a fifth on wpctl's cubic scale, which is 0.8% of the sound.
 *
 * Each was fixed, and the next one arrived. So it is not done that way any
 * more. What is lowered now is the stream's software gain — PipeWire's
 * softVolumes — which the stream applies and nobody saves. The volume its
 * owner chose, channelVolumes, is never touched, so WirePlumber has nothing to
 * remember, and the worst a crash in the middle of a sentence can do is leave
 * one stream quiet until it ends. Nothing here can outlive the stream it
 * touched, whatever goes wrong. See savedlevels.go for the one place a saved
 * level is set, which is putting right what the old way left behind.
 */
const (
	// DuckedTo is the share of its own level everything else keeps while the
	// brain speaks: a real fifth now, on a linear gain, so a voice sits clearly
	// on top and the music is still plainly playing.
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
 * stream is one thing playing, as far as turning it down goes.
 *
 * Known by serial rather than by id. An id is reused as soon as its stream
 * ends; a serial is never reused while PipeWire runs, so putting a level back
 * on a serial cannot land on some other program's stream that happened to
 * inherit the number.
 */
type stream struct {
	ID     int
	Serial string
	Name   string

	// Volumes is the level the stream's owner chose, one per channel. Read,
	// never written.
	Volumes []float64

	// Gain is what the stream is actually applying, one per channel.
	Gain []float64

	// Running is whether it is making a noise rather than paused.
	Running bool

	// Process is the process that opened it.
	Process int
}

var (
	duckMu   sync.Mutex
	lowered  map[string]stream // by serial: exactly the streams this turned down
	speaking int
	restorer *time.Timer
)

/*
 * duckOthers lowers everything else playing and returns a way to let it back
 * up.
 *
 * Counted rather than a plain on and off, because sentences overlap: the next
 * one is being generated while the last is still being heard, and an
 * unbalanced pair would leave somebody's music down for the rest of the
 * answer.
 */
func duckOthers(ctx context.Context) func() {
	if !haveGainControl() {
		return func() {}
	}

	duckMu.Lock()

	speaking++

	if restorer != nil {
		restorer.Stop()
		restorer = nil
	}

	first := speaking == 1 && lowered == nil

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

// lower takes down everything this machine is playing that is not the brain.
func lower(ctx context.Context) {
	found := map[string]stream{}

	for _, s := range streamsNow(ctx) {
		if !s.Running || len(s.Volumes) == 0 {
			continue
		}

		if putGain(ctx, s, scaled(s.Volumes, DuckedTo)) {
			found[s.Serial] = s
		}
	}

	duckMu.Lock()

	if lowered == nil {
		lowered = found
	} else {
		for serial, s := range found {
			lowered[serial] = s
		}
	}

	/*
	 * It may have stopped talking while this was working.
	 *
	 * Lowering is not instant — it reads the whole graph and then sets a gain
	 * per stream, which on a busy machine takes longer than the hold before
	 * the restore. When that happened the restore ran first, found nothing
	 * recorded yet and did nothing, and the music stayed down for as long as
	 * the brain went on thinking. So the check is made here, after the work,
	 * against the same lock the release uses.
	 */
	stopped := speaking == 0

	duckMu.Unlock()

	if stopped {
		restore()
	}
}

/*
 * restore puts every stream this lowered back to its own level.
 *
 * Its own context, not the turn's: by the time this runs the turn is over and
 * its context is cancelled. And nothing is kept for later: a stream that has
 * ended took its gain with it, and a stream still playing is set back now.
 * There is no unfinished job to write down, which is the point.
 */
func restore() {
	duckMu.Lock()

	was := lowered
	lowered = nil
	restorer = nil

	duckMu.Unlock()

	if len(was) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, s := range streamsNow(ctx) {
		if _, touched := was[s.Serial]; !touched || len(s.Volumes) == 0 {
			continue
		}

		// Back to the level its owner has now, not the one it had when it was
		// lowered: somebody may have changed it in the meantime.
		putGain(ctx, s, s.Volumes)
	}
}

/*
 * The room, as two functions, so a test can supply one.
 *
 * The tests around this used to read the real machine, and whether "nothing
 * was left turned down" came out true depended on whether a browser happened
 * to be playing while the suite ran.
 */
var (
	streamsNow = otherStreams
	putGain    = setGain
)

// PutTheVolumeBack restores anything turned down. Called when speech is
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

// haveGainControl reports whether this machine has the tools for it. Without
// them everything here does nothing, quietly, which is right: a machine with
// no PipeWire still has to be able to talk.
var haveGainControl = func() bool {
	for _, tool := range []string{"pw-dump", "pw-cli"} {
		if _, err := exec.LookPath(tool); err != nil {
			return false
		}
	}

	return true
}

/*
 * otherStreams lists the output streams on this machine that are not the
 * brain's own, paused ones included — a paused stream still exists, and a
 * gain left down on it is heard the moment it plays again.
 */
func otherStreams(ctx context.Context) []stream {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(ctx, "pw-dump").Output()
	if err != nil {
		return nil
	}

	var out []stream

	for _, s := range readStreams(raw) {
		if !s.mine {
			out = append(out, s.stream)
		}
	}

	return out
}

// seenStream is a stream as read, with whether it turned out to be ours.
type seenStream struct {
	stream
	mine bool
}

/*
 * readStreams reads the output streams out of pw-dump.
 *
 * Which process opened a stream is not on the stream. It is on the client the
 * stream belongs to, which the stream names by client.id — so the clients are
 * read first and the streams joined to them. Reading it off the stream found
 * nothing, silently, and every stream looked as though nobody had started it.
 */
func readStreams(raw []byte) []seenStream {
	found, err := dumpObjects(raw)
	if err != nil {
		return nil
	}

	type object struct {
		ID   int    `json:"id"`
		Type string `json:"type"`
		Info struct {
			State  string         `json:"state"`
			Props  map[string]any `json:"props"`
			Params struct {
				Props []struct {
					ChannelVolumes []float64 `json:"channelVolumes"`
					SoftVolumes    []float64 `json:"softVolumes"`
				} `json:"Props"`
			} `json:"params"`
		} `json:"info"`
	}

	objects := make([]object, 0, len(found))
	processOf := map[string]int{}

	for _, one := range found {
		var o object

		if err := json.Unmarshal(one, &o); err != nil {
			continue
		}

		objects = append(objects, o)

		if o.Type == "PipeWire:Interface:Client" {
			pid, _ := strconv.Atoi(text(o.Info.Props["application.process.id"]))
			processOf[strconv.Itoa(o.ID)] = pid
		}
	}

	var out []seenStream

	for _, object := range objects {
		props := object.Info.Props

		if text(props["media.class"]) != "Stream/Output/Audio" {
			continue
		}

		s := stream{
			ID:      object.ID,
			Serial:  text(props["object.serial"]),
			Name:    text(props["application.name"]),
			Running: object.Info.State == "running",
		}

		if s.Name == "" {
			s.Name = text(props["node.name"])
		}

		// The first Props object that carries the volumes; a node lists more
		// than one, and only one of them has these.
		for _, p := range object.Info.Params.Props {
			if len(p.ChannelVolumes) > 0 {
				s.Volumes = p.ChannelVolumes
				s.Gain = p.SoftVolumes

				break
			}
		}

		if s.Serial == "" {
			s.Serial = strconv.Itoa(s.ID)
		}

		s.Process = processOf[text(props["client.id"])]

		if s.Process == 0 {
			s.Process, _ = strconv.Atoi(text(props["application.process.id"]))
		}

		out = append(out, seenStream{stream: s, mine: ours(s.Name) || startedByMe(s.Process)})
	}

	return out
}

// text reads a property that pw-dump may print as a string or a number.
func text(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

/*
 * ours reports whether a stream is the brain's own voice, by name.
 *
 * The second of two checks, and the weaker: see startedByMe. A name list is
 * how the brain came to turn its own voice down — espeak calls its stream
 * "eSpeak", which was not on it — so it stays only for the voices that are
 * not the brain's children, like speech-dispatcher's.
 */
func ours(name string) bool {
	if isOwnAudio(name) {
		return true
	}

	folded := strings.ToLower(name)

	for _, mine := range []string{"pw-play", "piper", "speech-dispatcher", "espeak"} {
		if strings.Contains(folded, mine) {
			return true
		}
	}

	return false
}

/*
 * startedByMe reports whether a process is this program or one it started.
 *
 * Every sound the brain makes comes from a process it runs — espeak, piper's
 * player, aplay — and PipeWire records which process opened each stream. So
 * "is this our voice" is a question about the process tree, which has one
 * right answer, rather than about what an engine chose to call its stream.
 */
func startedByMe(pid int) bool {
	me := os.Getpid()

	for depth := 0; pid > 1 && depth < 12; depth++ {
		if pid == me {
			return true
		}

		pid = parentOf(pid)
	}

	return false
}

// parentOf is a process's parent, from /proc; 0 where that cannot be read.
var parentOf = func(pid int) int {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}

	// The command name is in brackets and may itself contain spaces and
	// brackets, so the fields are counted from after the last one.
	line := string(raw)
	end := strings.LastIndexByte(line, ')')

	if end < 0 {
		return 0
	}

	fields := strings.Fields(line[end+1:])

	if len(fields) < 2 {
		return 0
	}

	parent, _ := strconv.Atoi(fields[1])

	return parent
}

func scaled(levels []float64, by float64) []float64 {
	out := make([]float64, len(levels))

	for i, v := range levels {
		out[i] = v * by
	}

	return out
}

/*
 * gainArgument is the Props object pw-cli is handed: softVolumes and nothing
 * else.
 *
 * Nothing else is the rule. channelVolumes, volume and mute are what
 * WirePlumber saves against the program; softVolumes is not, which is the
 * difference between a level that ends with the stream and one that is still
 * there next week.
 */
func gainArgument(levels []float64) string {
	parts := make([]string, len(levels))

	for i, v := range levels {
		// Clamped: this is arithmetic on numbers read from another program,
		// and the failure at the top end is somebody being deafened.
		if v < 0 {
			v = 0
		}

		if v > 1.5 {
			v = 1.5
		}

		parts[i] = strconv.FormatFloat(v, 'f', 4, 64)
	}

	return "{ softVolumes: [ " + strings.Join(parts, ", ") + " ] }"
}

func setGain(ctx context.Context, s stream, levels []float64) bool {
	if len(levels) == 0 {
		return false
	}

	return exec.CommandContext(ctx, "pw-cli", "set-param", strconv.Itoa(s.ID), "Props",
		gainArgument(levels)).Run() == nil
}

/*
 * Whether to turn the rest down at all.
 *
 * A package-level switch rather than a parameter threaded through every
 * caller: speaking happens from a dozen places — the greeting, a reminder, an
 * answer, a test — and every one of them would have to be told.
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
