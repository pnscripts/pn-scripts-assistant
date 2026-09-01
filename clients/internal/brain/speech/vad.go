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

	/*
	 * SilenceToEnd is how long quiet must last before a turn is considered
	 * over.
	 *
	 * A comma is around 300ms and a full stop nearer 700, and 800ms was set
	 * from those numbers. It cuts people off. Those figures describe reading
	 * aloud from a page, and nobody talking to an assistant is reading: they
	 * are composing, and composing pauses are far longer — after a name, in
	 * the middle of a list, and above all before the part they actually
	 * wanted to say.
	 *
	 * 1.3s, and the delay it adds to a finished sentence is paid back by
	 * continuation, which reopens the microphone when the words themselves say
	 * somebody had not finished. Between them the fixed timeout stops being
	 * the thing that decides; see SoundsUnfinished.
	 */
	SilenceToEnd = 1300 * time.Millisecond

	/*
	 * PatienceForMore is how long to wait for somebody to carry on.
	 *
	 * Much shorter than the wait for a turn to begin. By this point they have
	 * already been speaking and the transcript stopped mid-thought, so either
	 * they resume promptly or they had finished and whisper simply did not
	 * punctuate it. Waiting the full eight seconds to find that out would make
	 * every unpunctuated sentence feel like a hang.
	 */
	PatienceForMore = 1500 * time.Millisecond

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
	/*
	 * NoiseMargin is how far above the room a frame must sit to be speech.
	 *
	 * Three and a half was chosen against a room whose noise floor was a fan,
	 * and it works perfectly there. It fails completely in the room this is
	 * actually for: with a film playing, the floor is the film — measured at
	 * 1400 — and somebody talking over it reaches 2500 to 4600, which is a
	 * clear one and a half to three times the room and nowhere near three and
	 * a half. The bar sat at 5000 and every single word went under it.
	 *
	 * A fan is steady, so it never beats even a small margin. A film is not
	 * steady, so it will beat this one and be transcribed — but the name is
	 * what decides whether anything is answered, and a line of dialogue that
	 * gets written down and thrown away costs a little processor time and
	 * nothing else. Being unable to hear the person is the worse failure by a
	 * long way.
	 */
	NoiseMargin = 1.7

	/*
	 * MinSpeechFloor stops a silent input calibrating so low that its own hiss
	 * registers as talking.
	 *
	 * It has to be a number rather than a multiple, because a multiple of
	 * almost-nothing is almost-nothing. But it was 500, which was measured
	 * against a loud room, and in a quiet one it is the only rule that applies:
	 * with the room at 40 it demanded speech twelve times louder than the room,
	 * when the margin everywhere else in this file is 1.7.
	 *
	 * The numbers that set this: a quiet room here measures 36 to 55, and
	 * somebody talking at an ordinary volume a little away from the microphone
	 * peaks at 300 to 630. Every word of that went under 500 and the brain
	 * reported hearing nothing at all. At 150 the room is still four times
	 * below the bar and ordinary speech is comfortably above it.
	 */
	MinSpeechFloor = 150

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

	/*
	 * BargeMargin is how far above its own voice somebody has to be to cut in.
	 *
	 * Measured against what the microphone hears while the brain is speaking,
	 * not against the room, because with speakers those are very different
	 * numbers. Too low and the brain interrupts itself on its own echo, which
	 * ends every answer after one sentence; too high and cutting in means
	 * shouting. Twice its own level is a person leaning in and talking over it,
	 * which is exactly the gesture this is for.
	 */
	BargeMargin = 2.0
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

	/*
	 * CutInAt is where in the file the person started talking over the
	 * assistant, or zero when they did not.
	 *
	 * Everything before it is the assistant's own voice, recorded while it was
	 * still speaking. Transcribing the whole file hands the recogniser both
	 * voices at once and it returns a muddle of the two — which is why talking
	 * over an answer produced either nothing or somebody else's sentence.
	 */
	CutInAt int64

	/*
	 * WasSteadyNoise marks a turn that crossed the level threshold without
	 * ever sounding like a voice — a cooling fan, traffic, a passing lorry.
	 *
	 * Reported rather than silently dropped, so the interface can say what
	 * actually happened instead of showing yet another turn that heard
	 * nothing and leaving its owner to wonder why.
	 */
	WasSteadyNoise bool
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
	return RecordTurnWaiting(ctx, device, path, PatienceBeforeSpeech)
}

/*
 * RecordTurnWaiting records one turn, waiting a given time for it to begin.
 *
 * The wait differs by situation and nothing else does. Opening a fresh turn is
 * patient, because somebody has to gather a thought; carrying one on is not,
 * because they were talking a second ago and either resume or had finished.
 */
func RecordTurnWaiting(
	ctx context.Context, device, path string, patience time.Duration,
) (Turn, error) {
	turn := Turn{Path: path}

	// Every frame once speech has been heard, for the whole-turn check below.
	var wholeTurn []int

	/*
	 * With nothing chosen, work out the best input on this machine.
	 *
	 * That means the echo-cancelled source when it is running and wired to the
	 * microphone actually in use, since that is what makes interrupting work:
	 * the assistant's own voice is subtracted from what the microphone hears,
	 * so it can keep listening while it speaks instead of going deaf for the
	 * length of every answer. When the canceller is missing or pointed
	 * somewhere else, the plain microphone — being able to hear at all is
	 * worth more than being able to interrupt.
	 */
	if device == "" {
		device = PreferredMicrophone(ctx)
	}

	/*
	 * One recorder on the microphone at a time.
	 *
	 * Two were found running together — one on the echo-cancelled source and
	 * one that had failed to resolve a device at all and was taking the raw
	 * default. Two processes pulling from one microphone split the audio
	 * between them, and each transcribes half a sentence: "Brain, tuberously"
	 * out of a whole spoken question, which then gets answered as though
	 * somebody had said it.
	 *
	 * Refused rather than queued. A turn that waits for the previous one is a
	 * turn recorded after the person stopped talking, which is not a recovery
	 * — and overlapping turns are not a thing that should be happening, so the
	 * honest response is to say so rather than to smooth it over.
	 */
	if !claimTheMicrophone() {
		return turn, ErrAlreadyRecording
	}

	defer releaseTheMicrophone()

	cmd, err := turnRecorder(ctx, device, path)
	if err != nil {
		return turn, err
	}

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

		// What the microphone hears while the brain itself is talking, which
		// is what somebody cutting in has to be louder than.
		echo []int
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
			/*
			 * Unless the room has proved it cannot be done here.
			 *
			 * With the microphone next to the speaker the echo arrives loud
			 * enough to clip and distorted in ways no canceller models, and
			 * enough of it survives to be transcribed as speech every time the
			 * assistant opens its mouth. Each of those costs a whole turn: the
			 * microphone was busy, the recogniser ran, and the person in front
			 * of it was not being listened to.
			 *
			 * So after a few of them, listening waits until it has stopped
			 * talking. That costs interrupting by voice and nothing else —
			 * the Stop button and Escape still work — and it recovers on its
			 * own once the echoes age out, so moving the microphone or turning
			 * the volume down brings the voice back without anybody finding a
			 * setting.
			 */
			if RoomIsTooLive() {
				started = time.Now()
				recent = recent[:0]
				echo = echo[:0]
				threshold = math.MaxInt

				continue
			}

			/*
			 * Listening while it talks, so it can be interrupted.
			 *
			 * This used to throw the frame away and hold the turn at its
			 * beginning, which is why an answer once started was always going
			 * to be finished: on this machine that can be a minute of speech,
			 * and sitting through a wrong answer to its end before being able
			 * to say so is not a conversation.
			 *
			 * The microphone hears both voices and there is no echo
			 * cancellation here, so what it hears while speaking is measured
			 * and the person has to be clearly above it. On headphones that
			 * measurement is only the room and anything said cuts in; on
			 * speakers it is the brain's own voice, and cutting in means
			 * actually talking over it.
			 */
			echo = append(echo, rms)

			if len(echo) > FloorWindow {
				echo = echo[len(echo)-FloorWindow:]
			}

			if len(echo) >= CalibrationFrames && rms > loudestEcho(echo)*BargeMargin {
				Interrupt()

				/*
				 * And the recording starts here too.
				 *
				 * Everything captured up to this point is the assistant
				 * talking. Keeping it means handing the recogniser two voices
				 * over the top of each other, and what comes back is a muddle
				 * of both — which is why cutting in produced either nothing or
				 * a sentence nobody said.
				 *
				 * A little before the moment it was noticed, because
				 * noticing takes a few frames and the first word is the one
				 * that gets lost.
				 */
				turn.CutInAt = offset - int64(bytesFor(3*FrameDuration))
				if turn.CutInAt < header {
					turn.CutInAt = header
				}

				// The turn starts here, with the interruption as its first
				// sound, so nothing said while cutting in is lost.
				started = time.Now().Add(-WarmUp)
				recent = recent[:0]
				threshold = MinSpeechFloor

				continue
			}

			started = time.Now()
			recent = recent[:0]
			echo = echo[:0]
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

		// Every frame of the turn once speech has been heard, so the whole of
		// it can be judged when it ends. See the check below.
		if turn.HeardSpeech {
			wholeTurn = append(wholeTurn, rms)
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

		/*
		 * Loud enough, and shaped like a voice.
		 *
		 * Loudness alone starts turns on the cooling fan: a machine at full
		 * load — which is every machine running a model on its processor —
		 * produces broadband noise whose peaks sit well above its own average,
		 * and that is all a threshold asks for. A recording kept from one such
		 * turn held twenty-three seconds at a steady RMS with no gaps, and
		 * whisper described it as "(engine revving)".
		 *
		 * Only applied once a turn is under way, never to end one: somebody
		 * pausing mid-sentence produces a flat stretch too, and using this to
		 * decide they had finished would cut them off exactly where they were
		 * thinking.
		 */
		speaking := rms >= threshold &&
			(turn.HeardSpeech || soundsLikeAVoice(recent))

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

			if time.Since(quietSince) >= patienceFor(spoke) && spoke >= MinSpeechDuration {
				turn.SpokeFor = spoke
				stop()

				/*
				 * One last look at the whole turn.
				 *
				 * The check that starts a turn has to give the benefit of the
				 * doubt: there is barely a second of history to judge by, and
				 * refusing on thin evidence means ignoring somebody. A fan
				 * slips through that gap and then sustains the turn for
				 * twenty seconds, because once speech has been heard the
				 * check is never applied again — measured at a swing of 2.4
				 * across eighteen seconds, where real speech reaches fifty.
				 *
				 * Here the turn is over and there is nothing left to cut off,
				 * so the whole recording can be judged at once.
				 */
				if !wholeTurnSoundsLikeAVoice(wholeTurn) {
					turn.HeardSpeech = false
					turn.WasSteadyNoise = true
				}

				return turn, nil
			}
		default:
			// Nobody has started yet.
			if time.Since(started) >= patience {
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

/*
 * loudestEcho is how loud the brain's own voice comes back.
 *
 * A high percentile rather than the maximum: one clipped frame of its own
 * output would otherwise set the bar for interrupting far above anything a
 * person can do, and the failure would look like the microphone ignoring them.
 */
func loudestEcho(frames []int) int {
	sorted := append([]int(nil), frames...)
	sort.Ints(sorted)

	at := len(sorted) * 9 / 10

	if at >= len(sorted) {
		at = len(sorted) - 1
	}

	// A floor, so a silent stretch of its own speech does not make every rustle
	// an interruption.
	if sorted[at] < MinSpeechFloor {
		return MinSpeechFloor
	}

	return sorted[at]
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

/*
 * turnRecorder is the command that captures one turn.
 *
 * PipeWire is the common case and the better one, since it can name a
 * particular microphone in a way that survives the device being unplugged and
 * plugged back in. But it is not everywhere — a minimal install, a server, an
 * older distribution or one that stayed with PulseAudio all have ALSA and no
 * pw-record — and on those machines conversation mode used to refuse to start
 * at all while one-shot listening worked fine, which is a strange thing for a
 * program to say when it can plainly hear.
 *
 * Both write a growing WAV, so the loop that watches the levels does not care
 * which one produced it. Neither finishes the header when interrupted, which
 * is what RepairWAV exists for.
 */
func turnRecorder(ctx context.Context, device, path string) (*exec.Cmd, error) {
	if _, err := exec.LookPath("pw-record"); err == nil {
		args := []string{"--rate", "16000", "--channels", "1", "--format", "s16"}

		if device != "" {
			args = append(args, "--target", device)
		}

		return exec.CommandContext(ctx, "pw-record", append(args, path)...), nil
	}

	if _, err := exec.LookPath("arecord"); err != nil {
		return nil, fmt.Errorf(
			"no way to record: neither pw-record nor arecord is installed")
	}

	args := []string{"-q", "-f", "S16_LE", "-r", "16000", "-c", "1"}

	/*
	 * A PipeWire node name means nothing to ALSA.
	 *
	 * The two name devices in entirely different ways, and handing one an
	 * identifier from the other makes arecord fail with a message about an
	 * unknown PCM that reads, to anyone who has not seen it before, like the
	 * microphone being broken. Falling back to the default input is the honest
	 * move: on a machine without PipeWire, ALSA's default is not being
	 * second-guessed by anything.
	 */
	if device != "" && alsaWouldUnderstand(device) {
		args = append(args, "-D", device)
	}

	return exec.CommandContext(ctx, "arecord", append(args, path)...), nil
}

/*
 * patienceFor is how long to wait in silence before calling a turn finished.
 *
 * Not one number, because one number cannot be right. Somebody asking a short
 * question stops and wants an answer immediately; somebody explaining
 * something is halfway through a thought and pauses to assemble the next part,
 * and those pauses get longer the longer they have been talking — after a
 * name, in the middle of a list, and above all just before the part they
 * actually meant to say.
 *
 * A fixed 800ms cut people off mid-sentence. 1300ms cut them off less often
 * and still did. So the wait grows with how long they have been speaking,
 * which costs a quick question nothing and gives a long one room.
 *
 * The upper bound matters as much as the slope: past two seconds of silence,
 * waiting longer stops feeling like patience and starts feeling like the
 * program has not noticed.
 */
func patienceFor(spoke time.Duration) time.Duration {
	const (
		grows = 2500 * time.Millisecond
		most  = 2200 * time.Millisecond
	)

	if spoke < grows {
		return SilenceToEnd
	}

	// Half a second more for every further two and a half seconds of talking.
	extra := time.Duration(spoke/grows) * 450 * time.Millisecond

	if wait := SilenceToEnd + extra; wait < most {
		return wait
	}

	return most
}

// bytesFor is how many bytes of 16-bit mono audio at 16kHz a duration takes.
func bytesFor(d time.Duration) int {
	return int(d.Seconds() * 16000 * 2)
}
