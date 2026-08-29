package speech

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"sync"
	"syscall"
	"time"
)

// Conversation timings.
//
// These are the difference between talking to something and operating it. Too
// short a silence and it cuts you off mid-thought; too long and every exchange
// drags. The numbers below come from how people actually pause: a comma is
// around 300ms, a full stop nearer 700ms, so waiting about a second after
// speech ends catches the end of a sentence without waiting through the next.
const (
	// FrameDuration is how often the level is checked.
	FrameDuration = 100 * time.Millisecond

	// SilenceToEnd is how long quiet must last before a turn is considered over.
	//
	// A comma is around 300ms and a full stop nearer 700, so this has to sit
	// above the first and near the second: shorter and it interrupts mid
	// sentence, longer and every single turn carries the delay. 800ms ends a
	// finished sentence without waiting through the next.
	SilenceToEnd = 800 * time.Millisecond

	// MinSpeechDuration guards against a cough or a door closing ending the
	// turn immediately with nothing usable in it.
	MinSpeechDuration = 400 * time.Millisecond

	// PatienceBeforeSpeech is how long to wait for somebody to start. Long
	// enough to gather a thought, short enough that a mistaken click does not
	// leave the microphone open.
	PatienceBeforeSpeech = 8 * time.Second

	// MaxTurnDuration stops a turn that never falls quiet — a television in the
	// room, a fan close to the microphone.
	MaxTurnDuration = 45 * time.Second

	// WarmUp is audio ignored at the start.
	//
	// Opening a capture stream makes a click, and it is loud: measured at RMS
	// 1653 here against a speech threshold of 350. Without this the turn begins
	// by hearing itself start, decides somebody spoke, and ends 1.1 seconds
	// later having recorded nothing but the click.
	WarmUp = 500 * time.Millisecond
)

// Speech detection is calibrated, not fixed.
//
// A single threshold cannot work: room tone here measures RMS 679 on a
// microphone at full gain and under 100 on the same microphone at its default
// gain, so any constant is either deaf in one room or triggered by the fan in
// another. The noise floor is measured at the start of every turn instead, and
// speech is whatever rises clearly above it.
const (
	// NoiseMargin is how far above the floor a frame must sit to count as
	// speech. Speech is several times louder than room tone; a fan is not.
	NoiseMargin = 3.5

	// MinSpeechFloor stops a silent input from calibrating so low that its own
	// hiss registers as talking.
	MinSpeechFloor = 500

	// CalibrationFrames is how many frames must be in hand before the loop is
	// willing to decide anything. Half a second.
	CalibrationFrames = 5

	/*
	 * FloorWindow and FloorPercentile are how the room is measured.
	 *
	 * Not from the first half-second, which is what this used to do and which
	 * fails in the one case that matters: somebody who says the name the moment
	 * the microphone opens. Their voice becomes the measurement, the bar is set
	 * three and a half times above their own speech, and nothing they say for
	 * the rest of the turn can ever clear it. Measured in the room this was
	 * written in: floor 1917, threshold 6709, peak 3741 — a turn that heard a
	 * person perfectly well and reported hearing nothing.
	 *
	 * Taking one of the quietest frames of a rolling window cannot fail that
	 * way. Speech is not continuous — there are gaps between words, between
	 * sentences, and to breathe — so the quiet handful of the last few seconds
	 * is the room, whether or not anybody was talking when the microphone
	 * opened. It also recovers on its own: the estimate falls as soon as the
	 * gaps arrive, instead of being fixed for the turn by whatever the first
	 * half-second happened to contain.
	 *
	 * A count rather than a proportion, because a proportion has to assume how
	 * much of the window is speech and is wrong whenever somebody talks more
	 * densely than assumed. This only needs the room to be audible three times
	 * in four seconds, which is a weaker thing to require.
	 */
	FloorWindow = 40
	FloorQuiet  = 3
)

// Turn is what one spoken turn amounted to.
type Turn struct {
	Path        string
	SpokeFor    time.Duration
	PeakRMS     int
	HeardSpeech bool

	// NoiseFloor and Threshold are what the room measured and what was
	// therefore required to count as speech. Reported because "it did not hear
	// me" is unanswerable without them.
	NoiseFloor int
	Threshold  int
}

// RecordTurn records until the speaker stops, rather than for a fixed time.
//
// This is what makes a conversation possible. A fixed six-second window forces
// somebody to pace their sentence to a timer, and cuts off anything longer —
// which is not talking, it is dictating into a stopwatch.
//
// The level is read from the file as pw-record writes it, so no extra process
// or pipe is needed and the recording is already on disk when the turn ends.
func RecordTurn(ctx context.Context, device, path string) (Turn, error) {
	turn := Turn{Path: path}

	if _, err := exec.LookPath("pw-record"); err != nil {
		return turn, fmt.Errorf("pw-record is needed for conversation mode")
	}

	args := []string{"--rate", "16000", "--channels", "1", "--format", "s16"}

	if device != "" {
		args = append(args, "--target", device)
	}

	cmd := exec.CommandContext(ctx, "pw-record", append(args, path)...)
	dieWithParent(cmd)

	if err := cmd.Start(); err != nil {
		return turn, fmt.Errorf("could not start recording: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Deferred as well as called explicitly.
	//
	// Every ordinary way out of the loop below stops the recorder on its way,
	// and each of those calls is where it belongs — the turn is over at that
	// point, not when the function happens to return. But relying on that alone
	// means any path added later that forgets leaves a recorder running with
	// the microphone open, which is a quiet failure nobody notices until the
	// machine is full of them. Once is what makes both safe.
	var once sync.Once

	stop := func() {
		once.Do(func() {
			cmd.Process.Signal(syscall.SIGINT)

			select {
			case <-done:
			case <-time.After(3 * time.Second):
				cmd.Process.Kill()
				<-done
			}
		})
	}

	defer stop()

	const header = 44

	var (
		offset      int64 = header
		speechSince time.Time
		quietSince  time.Time
		started     = time.Now()

		recent    []int
		threshold = math.MaxInt

		// The quietest this turn ever found the room, for the next turn to
		// start from.
		lowestSeen = math.MaxInt
	)

	defer func() {
		if lowestSeen != math.MaxInt {
			rememberRoom(lowestSeen)
		}
	}()

	ticker := time.NewTicker(FrameDuration)
	defer ticker.Stop()

	// Every return below is an end of listening, so the meter is cleared here
	// rather than at each of them.
	defer clearLevel("mic")

	// And background learning holds off while this is open: it runs a model
	// call of its own, and on four cores the two together make transcription
	// slow for no reason the person waiting can see.
	recordingStarted()
	defer recordingStopped()

	for {
		select {
		case <-ctx.Done():
			stop()

			return turn, ctx.Err()
		case err := <-done:
			// pw-record exited on its own; whatever it wrote is the turn.
			return turn, err
		case <-ticker.C:
		}

		rms, next := frameRMS(path, offset)

		/*
		 * Every frame, when asked for.
		 *
		 * Off unless the variable is set. It is here because the fault that
		 * made the brain stop answering to its name could not be found from
		 * outside this loop: the room was being measured from somebody's own
		 * voice, and from a chair that is indistinguishable from a microphone
		 * that is not working. Frame level and frame size told the difference
		 * in one run, after two wrong guesses.
		 */
		if os.Getenv("PN_BRAIN_VAD_TRACE") != "" {
			fmt.Fprintf(os.Stderr, "TRACE rms=%d bytes=%d since=%dms\n",
				rms, next-offset, time.Since(started)/time.Millisecond)
		}

		offset = next

		// The interface shows this, so it is published every frame including
		// during warm-up and calibration: the meter is reporting what the
		// microphone hears, which is true regardless of whether this frame is
		// yet allowed to decide anything about the turn.
		//
		// The threshold goes with it. It is the level this room has already
		// been measured never to fall below, and without it a display cannot
		// tell the fan from a voice — which is the same problem this loop has,
		// solved the same way.
		floor := 0.0
		if threshold != math.MaxInt {
			floor = float64(threshold)
		}

		publishLevelWithFloor("mic", float64(rms), floor)

		// The brain must not transcribe its own voice.
		//
		// On speakers the microphone hears whatever the brain is saying, and
		// with nothing stopping it the reply gets treated as the next thing the
		// user said. The conversation loop already speaks and listens strictly
		// in turn, but that only covers the conversation: the greeting, a voice
		// sample and a typed reply are all spoken without waiting, and any of
		// them can overlap an open microphone. Being asked at the moment it
		// matters is more reliable than any of the places one could try to
		// arrange the turns.
		//
		// While the voice is playing the turn is held at its beginning: the
		// warm-up is kept open and the room measurement is thrown away, because
		// a noise floor measured with the brain talking over it would be far too
		// high and the user would then have to shout to be heard.
		if Speaking() {
			started = time.Now()
			recent = recent[:0]
			threshold = math.MaxInt

			continue
		}

		// Still settling: advance the read position so the click is not
		// re-examined later, but let it decide nothing.
		if time.Since(started) < WarmUp {
			continue
		}

		/*
		 * The room, measured continuously rather than once.
		 *
		 * Every frame goes into the window, including the loud ones: it is a
		 * percentile, so speech raises it only when speech is nearly all there
		 * is, and the gaps between words pull it back down within a second.
		 */
		recent = append(recent, rms)

		if len(recent) > FloorWindow {
			recent = recent[len(recent)-FloorWindow:]
		}

		// Nothing decided until there is enough of the room to judge by.
		if len(recent) < CalibrationFrames {
			continue
		}

		seen := quietest(recent, FloorQuiet)

		if seen < lowestSeen {
			lowestSeen = seen
		}

		turn.NoiseFloor = roomFloor(seen)
		threshold = speechThreshold(turn.NoiseFloor)
		turn.Threshold = threshold

		if rms > turn.PeakRMS {
			turn.PeakRMS = rms
		}

		speaking := rms >= threshold

		switch {
		case speaking:
			quietSince = time.Time{}

			if speechSince.IsZero() {
				speechSince = time.Now()
			}

			turn.HeardSpeech = true
		case turn.HeardSpeech:
			if quietSince.IsZero() {
				quietSince = time.Now()
			}

			spoke := time.Since(speechSince)

			if time.Since(quietSince) >= SilenceToEnd && spoke >= MinSpeechDuration {
				turn.SpokeFor = spoke
				stop()

				return turn, nil
			}
		default:
			// Nobody has started yet.
			if time.Since(started) >= PatienceBeforeSpeech {
				stop()

				return turn, nil
			}
		}

		if time.Since(started) >= MaxTurnDuration {
			turn.SpokeFor = time.Since(speechSince)
			stop()

			return turn, nil
		}
	}
}

// frameRMS reads whatever has been written since offset and reports its volume.
//
// Returns the new offset so the next call reads only what is new — the file
// grows continuously, and re-reading it whole every tenth of a second would
// turn a long turn into quadratic work.
func frameRMS(path string, offset int64) (int, int64) {
	f, err := os.Open(path)
	if err != nil {
		return 0, offset
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() <= offset {
		return 0, offset
	}

	length := info.Size() - offset

	// A frame is 1600 samples at 16kHz; anything much larger is a backlog and
	// only the most recent part describes what is being said now.
	const maxFrame = 16000 * 2

	if length > maxFrame {
		offset = info.Size() - maxFrame
		length = maxFrame
	}

	buf := make([]byte, length)

	n, err := f.ReadAt(buf, offset)
	if n <= 0 {
		return 0, offset
	}

	buf = buf[:n&^1]

	var sum float64

	for i := 0; i+1 < len(buf); i += 2 {
		v := float64(int16(binary.LittleEndian.Uint16(buf[i:])))
		sum += v * v
	}

	if len(buf) == 0 {
		return 0, offset + int64(n)
	}

	return int(math.Sqrt(sum / float64(len(buf)/2))), offset + int64(n)
}

// speechThreshold decides what counts as louder than the room.
func speechThreshold(floor int) int {
	level := int(float64(floor) * NoiseMargin)

	if level < MinSpeechFloor {
		return MinSpeechFloor
	}

	return level
}

/*
 * What this room measured last time anybody listened to it.
 *
 * A turn lasts eight seconds and the window is four, so somebody who talks
 * without a real gap for the whole of it leaves nothing quiet to measure, and
 * the estimate becomes their own voice again — rarer than measuring only the
 * first half second, but the same fault. The room, though, is the same room it
 * was ten seconds ago. Keeping the last good measurement means a turn that
 * cannot see the room can still use it.
 *
 * Updated once per turn rather than per frame. Per frame was the first attempt
 * and it does nothing: the loop calls this ten times a second, so any gradual
 * approach to a loud reading arrives within a second and the bar goes up while
 * the person is still talking.
 */
var remembered struct {
	mu    sync.Mutex
	floor int
}

// roomFloor is what to treat as the room, given what this turn can see of it.
//
// The lower of the two, because a quieter reading is always evidence: either
// the room is quieter than the last turn found, or this turn is looking at a
// gap the last one did not have.
func roomFloor(seen int) int {
	remembered.mu.Lock()
	defer remembered.mu.Unlock()

	if remembered.floor == 0 || seen < remembered.floor {
		return seen
	}

	return remembered.floor
}

// rememberRoom records what a finished turn found, for the next one to start
// from.
//
// Falls at once and rises by a quarter at most, so a room that has genuinely
// got louder is followed within a few turns while a single turn of wall-to-wall
// speech cannot raise the bar above the person speaking.
func rememberRoom(lowest int) {
	if lowest <= 0 {
		return
	}

	remembered.mu.Lock()
	defer remembered.mu.Unlock()

	switch {
	case remembered.floor == 0 || lowest < remembered.floor:
		remembered.floor = lowest
	default:
		if ceiling := remembered.floor * 5 / 4; lowest > ceiling {
			remembered.floor = ceiling
		} else {
			remembered.floor = lowest
		}
	}
}

// quietest returns the nth lowest reading, which is this room with nobody
// talking over it.
//
// Not the lowest: a single dropped frame reads as near silence, and taking it
// would put the bar on the floor and make every rustle a sentence. The third
// quietest needs three of them to agree.
func quietest(values []int, nth int) int {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]int(nil), values...)
	sort.Ints(sorted)

	at := nth - 1

	if at >= len(sorted) {
		at = len(sorted) - 1
	}

	if at < 0 {
		at = 0
	}

	return sorted[at]
}

// median is used rather than a mean so one stray click during calibration does
// not raise the bar for the whole turn.
func median(values []int) int {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]int(nil), values...)
	sort.Ints(sorted)

	return sorted[len(sorted)/2]
}
