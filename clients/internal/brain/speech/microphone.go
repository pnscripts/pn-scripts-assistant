package speech

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Microphone is an input the brain could listen through.
//
// ID is PipeWire's node.name, not its object id. They are different numbers for
// the same device — the USB microphone here is object 33 and serial 50 — and
// pw-record's --target reads a serial or a name, never an object id. Passing
// the id silently targeted a serial that did not exist, so recording fell back
// to the default input and every attempt captured the wrong device. The name is
// unambiguous and survives reconnection.
type Microphone struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

// Microphones lists the audio inputs on this machine.
//
// Asked of PipeWire rather than ALSA. On a modern desktop ALSA's "default"
// capture device is whatever PipeWire put there, which on this machine is the
// built-in analog jack — so recording from it produces near-silence while a USB
// microphone sits unused two devices away. That failure looks exactly like a
// broken recogniser, which is the wrong thing to go debugging.
func Microphones(ctx context.Context) ([]Microphone, error) {
	if _, err := exec.LookPath("pw-dump"); err != nil {
		return alsaMicrophones(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(ctx, "pw-dump").Output()
	if err != nil {
		return nil, fmt.Errorf("could not ask PipeWire for inputs: %w", err)
	}

	found, err := dumpObjects(raw)
	if err != nil {
		return nil, fmt.Errorf("could not read PipeWire's reply: %w", err)
	}

	var objects []struct {
		ID   int `json:"id"`
		Info struct {
			Props map[string]any `json:"props"`
		} `json:"info"`
	}

	for _, one := range found {
		var object struct {
			ID   int `json:"id"`
			Info struct {
				Props map[string]any `json:"props"`
			} `json:"info"`
		}

		// An object that will not parse is skipped rather than fatal: one
		// malformed entry must not cost somebody the whole list of their
		// microphones.
		if err := json.Unmarshal(one, &object); err != nil {
			continue
		}

		objects = append(objects, object)
	}

	defaultName := defaultSourceName(ctx)

	var out []Microphone

	for _, o := range objects {
		props := o.Info.Props

		if str(props["media.class"]) != "Audio/Source" {
			continue
		}

		nodeName := str(props["node.name"])
		if nodeName == "" {
			continue
		}

		label := str(props["node.description"])
		if label == "" {
			label = nodeName
		}

		out = append(out, Microphone{
			ID:      nodeName,
			Name:    label,
			Default: defaultName != "" && nodeName == defaultName,
		})
	}

	return out, nil
}

// defaultSourceName asks which input the desktop currently prefers.
//
// Reported rather than obeyed: the default is frequently wrong — an empty
// analog jack outranking a plugged-in USB microphone — so it is shown as a
// label and the choice is left to the person.
func defaultSourceName(ctx context.Context) string {
	if _, err := exec.LookPath("wpctl"); err != nil {
		return ""
	}

	raw, err := exec.CommandContext(ctx, "wpctl", "inspect", "@DEFAULT_AUDIO_SOURCE@").Output()
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, "node.name") {
			continue
		}

		if _, value, found := strings.Cut(line, "="); found {
			return strings.Trim(strings.TrimSpace(value), `"`)
		}
	}

	return ""
}

func str(v any) string {
	s, _ := v.(string)

	return s
}

/*
 * Which microphone to record from, on a machine nobody has described in
 * advance.
 *
 * Three things can each be wrong independently, and each one looks from the
 * outside like the microphone having stopped working:
 *
 *   - the desktop's default input is not the microphone anyone is speaking
 *     into, which is normal rather than unusual: an empty analog jack outranks
 *     a plugged-in USB microphone on this machine,
 *   - the echo canceller exists but is subtracting the assistant's voice from
 *     the wrong input, so it passes on a faint hiss,
 *   - the microphone that was chosen last time has since been unplugged.
 *
 * So nothing here is taken on faith. The canceller is used only once it has
 * been checked that it captures from the microphone actually in use, the
 * inputs are ranked by what they are rather than by what the desktop prefers,
 * and each candidate is listened to briefly before being trusted.
 */

// notAMicrophone matches inputs that exist but nobody speaks into.
//
// Monitors are the speakers' own output offered back as an input; recording one
// captures whatever is playing, which for this program means it hears itself
// perfectly and the person not at all.
func notAMicrophone(id string) bool {
	return strings.HasSuffix(id, ".monitor") ||
		strings.Contains(id, "echo_cancelled") ||
		strings.Contains(id, "echo-cancel")
}

// PhysicalMicrophones is the inputs someone could actually speak into.
func PhysicalMicrophones(ctx context.Context) []Microphone {
	mics, err := Microphones(ctx)
	if err != nil {
		return nil
	}

	var out []Microphone

	for _, m := range mics {
		if !notAMicrophone(m.ID) {
			out = append(out, m)
		}
	}

	return out
}

// echoCancelledSource is the canceller's output, if it is running at all.
func echoCancelledSource(ctx context.Context) string {
	mics, err := Microphones(ctx)
	if err != nil {
		return ""
	}

	for _, m := range mics {
		// By name, from the config this program writes, rather than by the
		// description — which is translated on a machine in another language
		// and would silently stop matching.
		if strings.Contains(m.ID, "echo_cancelled") || strings.Contains(m.ID, "echo-cancel") {
			return m.ID
		}
	}

	return ""
}

/*
 * cancellerHears reports which microphone the echo canceller captures from.
 *
 * Worth asking rather than assuming, because the answer was wrong here for a
 * while and nothing said so. Left without an explicit target, PipeWire links
 * the canceller to whatever it considers the default input — the empty analog
 * jack — and the canceller then dutifully cancels echo from a microphone
 * nobody is speaking into. Everything downstream still works: audio arrives,
 * levels are measured, recordings are written. They are just recordings of an
 * empty room, and the only visible symptom is that talking stops doing
 * anything.
 */
func cancellerHears(ctx context.Context) string {
	if _, err := exec.LookPath("pw-link"); err != nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(ctx, "pw-link", "-l").Output()
	if err != nil {
		return ""
	}

	/*
	 * pw-link lists a port, then its connections indented beneath it:
	 *
	 *     pn-brain.echo-cancel.capture:input_MONO
	 *       |<- alsa_input.usb-...Trust_GXT_232...:capture_MONO
	 *
	 * so the line after the canceller's input port names its source.
	 */
	return whatFeedsCanceller(string(raw))
}

// whatFeedsCanceller reads the source node out of `pw-link -l` output.
func whatFeedsCanceller(listing string) string {
	/*
	 * pw-link lists a port, then its connections indented beneath it:
	 *
	 *     pn-brain.echo-cancel.capture:input_MONO
	 *       |<- alsa_input.usb-...Trust_GXT_232...:capture_MONO
	 *
	 * so the line after the canceller's input port names its source. The port
	 * suffix differs by channel layout — input_MONO on a mono microphone,
	 * input_FL on a stereo one — which is why the prefix is matched and the
	 * rest of the port name is not.
	 */
	var atCapture bool

	for _, line := range strings.Split(listing, "\n") {
		if strings.HasPrefix(line, "pn-brain.echo-cancel.capture:") {
			atCapture = true

			continue
		}

		if !atCapture {
			continue
		}

		if _, from, found := strings.Cut(strings.TrimSpace(line), "|<- "); found {
			node, _, _ := strings.Cut(from, ":")

			return node
		}

		atCapture = false
	}

	return ""
}

func alive(ctx context.Context, id string) bool {
	if _, err := exec.LookPath("pw-record"); err != nil {
		return true // Cannot check, so do not condemn it.
	}

	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	sample, err := os.CreateTemp("", "pn-brain-probe-*.wav")
	if err != nil {
		return true
	}

	path := sample.Name()

	sample.Close()
	defer os.Remove(path)

	cmd := exec.CommandContext(ctx, "pw-record",
		"--rate", "16000", "--channels", "1", "--format", "s16",
		"--target", id, path)
	dieWithParent(cmd)

	if err := cmd.Start(); err != nil {
		return true
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(600 * time.Millisecond):
		cmd.Process.Signal(syscall.SIGINT)

		select {
		case <-done:
		case <-time.After(time.Second):
			cmd.Process.Kill()
			<-done
		}
	}

	// The recorder was killed, so the header still holds its placeholders.
	RepairWAV(path)

	raw, err := os.ReadFile(path)
	if err != nil || len(raw) < 44+320 {
		return false
	}

	var loudest int

	for i := 44; i+1 < len(raw); i += 2 {
		level := int(int16(uint16(raw[i]) | uint16(raw[i+1])<<8))
		if level < 0 {
			level = -level
		}

		if level > loudest {
			loudest = level
		}
	}

	// Digital silence is a dead input. Anything above it could be a room.
	return loudest > 12
}

/*
 * BestMicrophone ranks the inputs and returns the most likely one.
 *
 * The desktop's own default is treated as a weak hint rather than an answer.
 * It is chosen by rules that have nothing to do with this program — most
 * recently plugged in, or alphabetical order, or whatever a distribution
 * decided — and it is the reason the assistant spent this morning listening to
 * an empty jack.
 */
func BestMicrophone(ctx context.Context) string {
	ranked := rankMicrophones(PhysicalMicrophones(ctx))
	if len(ranked) == 0 {
		return ""
	}

	if len(ranked) == 1 {
		return ranked[0]
	}

	// Listened to in order, so a highly-ranked input that turns out to be
	// disconnected gives way to a humbler one that is actually working.
	for _, candidate := range ranked {
		if alive(ctx, candidate) {
			return candidate
		}
	}

	return ranked[0]
}

// rankMicrophones orders inputs by how likely each is to be the one in front of
// the person, most likely first.
//
// Separated from the listening so the ordering can be checked against machines
// this one will never be: a laptop with nothing attached, a desktop with a USB
// headset, a monitor offering an HDMI input that is not a microphone at all.
func rankMicrophones(mics []Microphone) []string {
	type scored struct {
		id    string
		score int
	}

	var ranked []scored

	for _, m := range mics {
		id := strings.ToLower(m.ID)
		name := strings.ToLower(m.Name)

		var score int

		/*
		 * Something someone chose to plug in beats something merely built in.
		 *
		 * A USB microphone or a headset is on the machine because a person put
		 * it there, and almost always to be spoken into. A built-in analog
		 * input is present whether or not anything is attached to it.
		 */
		switch {
		case strings.Contains(id, "usb"), strings.Contains(name, "headset"),
			strings.Contains(name, "microphone"):
			score += 40
		case strings.Contains(id, "bluez"), strings.Contains(id, "bluetooth"):
			score += 30
		}

		// A laptop's own microphone is a real one, and on a laptop with nothing
		// plugged in it is the only one. It just loses to anything attached.
		if strings.Contains(id, "analog") || strings.Contains(id, "internal") {
			score += 10
		}

		// HDMI and DisplayPort inputs are not microphones in any useful sense.
		if strings.Contains(id, "hdmi") || strings.Contains(id, "displayport") {
			score -= 50
		}

		if m.Default {
			score += 5
		}

		ranked = append(ranked, scored{id: m.ID, score: score})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	out := make([]string, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.id)
	}

	return out
}

/*
 * PreferredMicrophone is the input to record from when nobody has chosen one.
 *
 * The echo-cancelled source wins when it is available and correctly wired, and
 * it is the difference between an assistant that can be interrupted and one
 * that cannot: with its own output subtracted it can listen and speak at the
 * same time, the way a person on a telephone does.
 *
 * "Correctly wired" is the part that has to be checked rather than assumed.
 * A canceller pointed at the wrong input is strictly worse than no canceller
 * at all — it is quieter, noisier, and it hides which microphone it settled on
 * behind a name that still says echo cancelled.
 */
func PreferredMicrophone(ctx context.Context) string {
	best := BestMicrophone(ctx)

	cancelled := echoCancelledSource(ctx)
	if cancelled == "" {
		return best
	}

	// Wired to the microphone in use, so it is both cancelled and audible.
	if hears := cancellerHears(ctx); hears != "" && hears == best {
		return cancelled
	}

	/*
	 * Otherwise the raw microphone, which is the safe half of the trade.
	 *
	 * Interrupting mid-sentence stops working until the canceller is set up
	 * again, and being able to hear at all matters more than being able to
	 * interrupt.
	 */
	return best
}

/*
 * EnsureEchoCancellation points the canceller at the microphone in use.
 *
 * Called at startup and whenever the chosen microphone changes, because the
 * right answer moves: a headset is plugged in, a USB microphone is unplugged,
 * and a canceller wired to yesterday's input goes on cancelling the echo of a
 * device that is no longer there.
 *
 * Best-effort by design. Every failure here costs the ability to interrupt and
 * nothing else, so a machine without the module, without PipeWire, or without
 * permission to restart its own audio still listens and still answers.
 */
func EnsureEchoCancellation(ctx context.Context) error {
	best := BestMicrophone(ctx)
	if best == "" {
		return fmt.Errorf("no microphone to cancel the echo from")
	}

	if hears := cancellerHears(ctx); hears == best {
		return nil
	}

	return SetUpEchoCancellation(ctx, best)
}

/*
 * alsaMicrophones lists inputs on a machine with no PipeWire.
 *
 * Not every machine this runs on has it. A minimal install, a server, an older
 * distribution or one that stayed with PulseAudio all have ALSA and nothing
 * else, and the honest answer there is a short list of cards rather than an
 * error saying inputs cannot be listed — which reads as "this program cannot
 * hear" when the microphone is sitting there working.
 *
 * The identifiers are ALSA's own (hw:0,0) because that is what arecord takes,
 * and turnRecorder is careful not to hand them to pw-record, which would not
 * know what to do with them.
 */
func alsaMicrophones(ctx context.Context) ([]Microphone, error) {
	if _, err := exec.LookPath("arecord"); err != nil {
		return nil, fmt.Errorf(
			"no way to list inputs: neither PipeWire nor ALSA tools are installed")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(ctx, "arecord", "-l").Output()
	if err != nil {
		return nil, fmt.Errorf("could not ask ALSA for inputs: %w", err)
	}

	/*
	 * arecord -l prints one card per stanza:
	 *
	 *     card 1: Microphone [Trust GXT 232 Microphone], device 0: USB Audio ...
	 */
	var out []Microphone

	for _, line := range strings.Split(string(raw), "\n") {
		match := alsaCard.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		out = append(out, Microphone{
			ID:   fmt.Sprintf("hw:%s,%s", match[1], match[3]),
			Name: match[2],
			// ALSA has a "default" but it is a name, not a flag on a card, and
			// claiming one of these is it would be a guess.
			Default: false,
		})
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no microphone found")
	}

	return out, nil
}

/*
 * alsaCard matches one line of `arecord -l`:
 *
 *     card 1: Microphone [Trust GXT 232 Microphone], device 0: USB Audio ...
 *
 * The bracketed name is the readable one; the card and device numbers are what
 * arecord takes back as hw:1,0.
 */
var alsaCard = regexp.MustCompile(`^card (\d+): [^\[]*\[([^\]]+)\], device (\d+):`)

/*
 * alsaWouldUnderstand reports whether a device name means anything to ALSA.
 *
 * PipeWire and ALSA name devices in entirely different ways, and a machine can
 * hold a saved PipeWire name from a time when it had PipeWire, or from settings
 * copied off another machine. Handed one of those, arecord fails with a message
 * about an unknown PCM — which reads, to anyone who has not seen it before,
 * like the microphone being broken rather than the name being in the wrong
 * dialect. Falling back to the default input is the honest move: on a machine
 * without PipeWire, ALSA's default is not being second-guessed by anything.
 */
func alsaWouldUnderstand(device string) bool {
	return !strings.Contains(device, "alsa_input.") &&
		!strings.Contains(device, "alsa_output.") &&
		!strings.Contains(device, "echo")
}

/*
 * PreferredSpeaker is where the assistant's own voice should be played.
 *
 * Not the ordinary speakers, when echo cancellation is running. A canceller
 * subtracts a reference signal from what the microphone hears, and the
 * reference is whatever was played *into its sink* — so sound sent straight to
 * the hardware output is sound the canceller never sees and therefore cannot
 * remove.
 *
 * That was the state of things: the microphone was correctly wired into the
 * canceller, the canceller was running, and the voice was being played past it
 * to the default output. So the assistant heard itself perfectly. Saying its
 * name while it was talking started a turn made of its own words, which reads
 * as the program interrupting itself.
 *
 * Returns empty when there is no such sink, and then the default output is
 * used exactly as before.
 */
func PreferredSpeaker(ctx context.Context) string {
	// Only worth routing through when the microphone side is also cancelled.
	// On its own it changes nothing and adds a device that can go wrong.
	if cancelled := echoCancelledSource(ctx); cancelled == "" {
		return ""
	}

	if echoSinkPresent(ctx) {
		return "pn_brain_echo_sink"
	}

	return ""
}

// echoSinkPresent reports whether the canceller's sink is running.
func echoSinkPresent(ctx context.Context) bool {
	if _, err := exec.LookPath("pw-dump"); err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	raw, err := exec.CommandContext(ctx, "pw-dump").Output()
	if err != nil {
		return false
	}

	var objects []struct {
		Info struct {
			Props map[string]any `json:"props"`
		} `json:"info"`
	}

	if err := json.Unmarshal(raw, &objects); err != nil {
		return false
	}

	for _, o := range objects {
		props := o.Info.Props

		if !strings.HasPrefix(str(props["media.class"]), "Audio/Sink") {
			continue
		}

		if str(props["node.name"]) == "pn_brain_echo_sink" {
			return true
		}
	}

	return false
}
