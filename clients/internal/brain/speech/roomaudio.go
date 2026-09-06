package speech

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

/*
 * Cancelling everything this machine plays, not only its own voice.
 *
 * The canceller subtracts from the microphone whatever was played into its
 * sink. The brain's own voice goes there, so the brain does not hear itself.
 * Nothing else does: a browser playing music, a video, a call — all of it goes
 * straight to the speakers, and the microphone hears every note of it mixed
 * with whoever is talking.
 *
 * What comes back from the recogniser is then a blend, and it is a confident
 * blend: song lyrics arrive as a question, and a question with music behind it
 * arrives as neither. Telling one voice from another does not help, because
 * the recording genuinely contains two things.
 *
 * So the machine's own output is routed through the canceller. Sound still
 * reaches the speakers — the canceller passes it through — and the microphone
 * stops hearing it. Music from a phone on the desk is still music in the room,
 * and nothing here can help with that; this is for what the machine itself
 * plays, which is most of it.
 *
 * Off unless somebody asks. It changes where every program on the machine
 * sends its sound, which is not a thing to do to somebody quietly, and it is
 * put back when the brain stops.
 */

// EchoSink is the sink the canceller listens to.
const EchoSink = "pn_brain_echo_sink"

var routing struct {
	mu       sync.Mutex
	restore  string
	changed  bool
	lastFail string
}

/*
 * CancelWhatThisMachinePlays points the system's default output at the
 * canceller.
 *
 * The previous default is remembered so it can be put back. If the brain stops
 * without putting it back — killed, crashed — the sink disappears and the
 * audio server falls back on its own, so the worst case is a moment of
 * confusion rather than a machine with no sound.
 */
func CancelWhatThisMachinePlays(ctx context.Context) error {
	routing.mu.Lock()
	defer routing.mu.Unlock()

	if routing.changed {
		return nil
	}

	if !haveMetadata() {
		return fmt.Errorf("this machine's audio server cannot be asked to reroute sound")
	}

	if !sinkExists(ctx, EchoSink) {
		return fmt.Errorf("the echo canceller is not running, so there is nothing to route through")
	}

	was := defaultSink(ctx)

	if was == EchoSink {
		// Already there, from a previous run that did not put it back.
		routing.changed = true

		return nil
	}

	if err := setDefaultSink(ctx, EchoSink); err != nil {
		return err
	}

	routing.restore = was
	routing.changed = true

	/*
	 * And whatever is already playing.
	 *
	 * A new stream follows the default; one that started before it does not,
	 * and the music somebody is complaining about is always already playing.
	 */
	moveEverythingPlaying(ctx)

	return nil
}

// PutTheSoundBack restores the output the machine had before.
func PutTheSoundBack(ctx context.Context) {
	routing.mu.Lock()
	defer routing.mu.Unlock()

	/*
	 * Where to put it back to, even when this run is not the one that moved it.
	 *
	 * It used to give up unless it had done the routing itself and remembered
	 * a sink. That is every case except the one that matters: the brain starts
	 * with the rerouting already switched on, routes the machine's sound at
	 * launch, and somebody then switches it off — and nothing was remembered
	 * from before the launch, so it did nothing and said nothing, leaving the
	 * whole machine playing into a sink with the film on it.
	 *
	 * Any real output that is not the canceller's own will do. It is where the
	 * sound was going before this program touched it.
	 */
	back := routing.restore

	if back == "" {
		back = anyRealSink(ctx)
	}

	if back == "" {
		return
	}

	/*
	 * Its own deadline, not the caller's.
	 *
	 * This is called from an HTTP handler, whose context is cancelled the
	 * moment the response is written — which killed pw-metadata halfway and
	 * left the machine pointing at the canceller, with the flag already
	 * cleared so nothing would try again.
	 */
	own, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()

	if err := setDefaultSink(own, back); err != nil {
		// Left as it was, so this can be tried again rather than being
		// recorded as done.
		return
	}

	moveEverythingPlaying(own)

	routing.changed = false
	routing.restore = ""
}

/*
 * anyRealSink is an output that is not the canceller's.
 *
 * The canceller's sink exists to be written into and read back from; sending
 * the machine's sound there when the canceller is off is sending it nowhere,
 * which is what a muted film sounds like.
 */
func anyRealSink(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(ctx, "pw-dump").Output()
	if err != nil {
		return ""
	}

	found, err := dumpObjects(raw)
	if err != nil {
		return ""
	}

	for _, one := range found {
		var object struct {
			Info struct {
				Props struct {
					Class string `json:"media.class"`
					Node  string `json:"node.name"`
				} `json:"props"`
			} `json:"info"`
		}

		if err := json.Unmarshal(one, &object); err != nil {
			continue
		}

		if object.Info.Props.Class != "Audio/Sink" {
			continue
		}

		if strings.Contains(object.Info.Props.Node, "pn_brain") ||
			strings.Contains(object.Info.Props.Node, "pn-brain") {
			continue
		}

		return object.Info.Props.Node
	}

	return ""
}

// Rerouting reports whether the machine's sound is currently going through the
// canceller because this program asked for it.
func Rerouting() bool {
	routing.mu.Lock()
	defer routing.mu.Unlock()

	return routing.changed
}

func haveMetadata() bool {
	_, err := exec.LookPath("pw-metadata")

	return err == nil
}

// defaultSink reads the name of the machine's current output.
func defaultSink(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "pw-metadata", "-n", "default").Output()
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "default.audio.sink") {
			continue
		}

		/*
		 * The value is JSON inside a quoted field: value:'{"name":"..."}'.
		 * Parsed rather than picked apart with string surgery, because the
		 * name can contain almost anything.
		 */
		open := strings.Index(line, "value:'")

		if open < 0 {
			continue
		}

		rest := line[open+len("value:'"):]
		close := strings.LastIndex(rest, "'")

		if close < 0 {
			continue
		}

		var named struct {
			Name string `json:"name"`
		}

		if json.Unmarshal([]byte(rest[:close]), &named) == nil {
			return named.Name
		}
	}

	return ""
}

func setDefaultSink(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	value, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return err
	}

	out, err := exec.CommandContext(ctx, "pw-metadata", "-n", "default", "0",
		"default.audio.sink", string(value), "Spa:String:JSON").CombinedOutput()
	if err != nil {
		return fmt.Errorf("pointing the machine's sound at %s: %w (%s)",
			name, err, strings.TrimSpace(string(out)))
	}

	return nil
}

func sinkExists(ctx context.Context, name string) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "pw-cli", "ls", "Node").Output()
	if err != nil {
		return false
	}

	return strings.Contains(string(out), `node.name = "`+name+`"`)
}

/*
 * moveEverythingPlaying puts streams that are already running onto the new
 * output.
 *
 * Best effort, and deliberately quiet about failures: a stream that refuses to
 * move is one program whose sound the microphone still hears, not a reason to
 * refuse the whole change.
 */
func moveEverythingPlaying(ctx context.Context) {
	if _, err := exec.LookPath("wpctl"); err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "wpctl", "status").Output()
	if err != nil {
		return
	}

	for _, id := range playingStreams(string(out)) {
		exec.CommandContext(ctx, "wpctl", "move", id, "@DEFAULT_AUDIO_SINK@").Run()
	}
}

/*
 * playingStreams pulls the stream ids out of wpctl's listing.
 *
 * The Streams section under Audio lists each one as an indented id followed by
 * the program's name. Everything above it is devices, which must not be moved.
 */
func playingStreams(listing string) []string {
	var (
		out      []string
		inAudio  bool
		inStream bool
	)

	for _, line := range strings.Split(listing, "\n") {
		trimmed := strings.TrimLeft(line, " │├└─\t")

		switch {
		case strings.HasPrefix(trimmed, "Audio"):
			inAudio = true
			inStream = false

			continue

		case strings.HasPrefix(trimmed, "Video"), strings.HasPrefix(trimmed, "Settings"):
			inAudio = false
			inStream = false

			continue

		case inAudio && strings.HasPrefix(trimmed, "Streams:"):
			inStream = true

			continue

		case inAudio && (strings.HasPrefix(trimmed, "Sinks:") ||
			strings.HasPrefix(trimmed, "Sources:") ||
			strings.HasPrefix(trimmed, "Filters:") ||
			strings.HasPrefix(trimmed, "Devices:")):
			inStream = false

			continue
		}

		if !inStream {
			continue
		}

		/*
		 * Blank lines sit inside every section of this listing, and a blank
		 * line has no first field. Asking for one anyway is how a parser that
		 * reads perfectly well takes the program down the first time somebody
		 * switches this on.
		 */
		fields := strings.Fields(trimmed)

		if len(fields) == 0 {
			continue
		}

		id := strings.TrimSuffix(fields[0], ".")

		if id == "" {
			continue
		}

		if _, err := fmt.Sscanf(id, "%d", new(int)); err == nil {
			out = append(out, id)
		}
	}

	return out
}

/*
 * Whether this machine was making a noise of its own.
 *
 * The third thing worth knowing about a recording, after how loud it was and
 * whose voice it is. A sentence heard while a browser is playing a video is
 * very likely the video: not somebody in the room, not the brain's own voice,
 * but the machine talking to itself through its own speakers.
 *
 * It is a different fact from the other two and it fixes a different mistake.
 * A voiceprint says "that was not you", which leaves open whether it was a
 * person; this says "something on this machine was playing at that moment",
 * which usually settles it.
 */

// PlayingItself reports whether any program other than the brain was sending
// sound to the speakers, and names them.
func PlayingItself(ctx context.Context) (bool, []string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	/*
	 * pw-dump first, because it is the only one of the two that says whether a
	 * stream is actually running.
	 *
	 * That distinction is the whole point here. A browser keeps its audio
	 * stream open while a video sits paused, and a text-to-speech daemon keeps
	 * one open from the moment it starts; reading only the list of streams
	 * would call both of those "playing" and answer every question about the
	 * room with a machine that is making no sound at all.
	 */
	if _, err := exec.LookPath("pw-dump"); err == nil {
		if out, err := exec.CommandContext(ctx, "pw-dump").Output(); err == nil {
			if programs, err := playingFromDump(out); err == nil {
				return len(programs) > 0, programs
			}
		}
	}

	// Otherwise the plain listing, which cannot tell paused from playing and
	// so errs towards saying something is.
	if _, err := exec.LookPath("pw-cli"); err != nil {
		return false, nil
	}

	out, err := exec.CommandContext(ctx, "pw-cli", "ls", "Node").Output()
	if err != nil {
		return false, nil
	}

	programs := playingPrograms(string(out))

	return len(programs) > 0, programs
}

/*
 * playingFromDump reads the running output streams out of a pw-dump.
 *
 * Only three things are wanted from a document that is a third of a megabyte:
 * what each stream is called, that it carries sound out, and that it is
 * running rather than merely open.
 */
func playingFromDump(dump []byte) ([]string, error) {
	found, err := dumpObjects(dump)
	if err != nil {
		return nil, err
	}

	type node struct {
		Info struct {
			State string `json:"state"`
			Props struct {
				Class       string `json:"media.class"`
				Application string `json:"application.name"`
				Node        string `json:"node.name"`
			} `json:"props"`
		} `json:"info"`
	}

	var nodes []node

	for _, one := range found {
		var n node

		if err := json.Unmarshal(one, &n); err != nil {
			continue
		}

		nodes = append(nodes, n)
	}

	var out []string

	for _, n := range nodes {
		if n.Info.Props.Class != "Stream/Output/Audio" || n.Info.State != "running" {
			continue
		}

		name := n.Info.Props.Application
		if name == "" {
			name = n.Info.Props.Node
		}

		out = appendPlaying(out, name)
	}

	return out, nil
}

/*
 * appendPlaying adds a name unless it is one of ours or already there.
 *
 * The brain's own players are left out. It plays its voice through the same
 * mechanism as everything else, and counting that would mean every answer it
 * gave was evidence that the machine was making a noise — which is true and
 * useless, since the canceller has already removed it.
 */
func appendPlaying(out []string, name string) []string {
	if name == "" {
		return out
	}

	ours := []string{"pn-brain", "pn_brain", "pw-play", "piper", "speech-dispatcher"}

	for _, mine := range ours {
		if strings.Contains(strings.ToLower(name), mine) {
			return out
		}
	}

	for _, already := range out {
		if already == name {
			return out
		}
	}

	return append(out, name)
}

/*
 * playingPrograms reads the names of the output streams out of a node listing.
 *
 * This is the fallback, for a machine with pw-cli but no pw-dump. The listing
 * carries no state, so an open-but-silent stream counts as playing — which is
 * the safer way round to be wrong, since it explains a confusion that is not
 * there rather than denying one that is.
 */
func playingPrograms(listing string) []string {
	var (
		out     []string
		name    string
		isSound bool
	)

	finish := func() {
		if !isSound {
			return
		}

		out = appendPlaying(out, name)
	}

	for _, line := range strings.Split(listing, "\n") {
		/*
		 * Each node is a block of properties, and a blank line or the start of
		 * the next object ends it. The two facts wanted — what it is called and
		 * whether it is playing sound — arrive in either order, so the block is
		 * finished rather than read in sequence.
		 */
		if strings.Contains(line, "id ") && strings.Contains(line, "type PipeWire:Interface:Node") {
			finish()

			name, isSound = "", false

			continue
		}

		if value := property(line, "media.class"); value != "" {
			isSound = value == "Stream/Output/Audio"
		}

		// application.name is the human one; node.name is the fallback for a
		// stream that did not give itself a name.
		if value := property(line, "application.name"); value != "" {
			name = value
		} else if value := property(line, "node.name"); value != "" && name == "" {
			name = value
		}
	}

	finish()

	return out
}

// property pulls the value out of a `key = "value"` line, or empty.
func property(line, key string) string {
	at := strings.Index(line, key+" = ")

	if at < 0 {
		return ""
	}

	rest := line[at+len(key)+3:]
	rest = strings.TrimSpace(rest)

	return strings.Trim(rest, `"`)
}
