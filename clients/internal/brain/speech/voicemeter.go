package speech

import (
	"encoding/binary"
	"math"
	"sync"
	"time"
)

// voiceMeter reports how loud the brain's own voice is, moment by moment.
//
// The obvious implementation is wrong, and it is worth saying why. Piper is
// piped straight into the player, so one could measure each chunk as it passes
// and publish that. But piper generates far faster than the sound plays — it
// finishes a sentence long before the sentence is heard, which is exactly why
// Speak waits on the player and not on the synthesiser. Metering the pipe would
// therefore run more than a second ahead of the audio, and the display would
// finish moving while the voice was still mid-word. It would look like a bug
// even though every number in it was measured.
//
// So the two jobs are separated. Chunks passing down the pipe are recorded into
// an envelope, indexed by their position in the sound. A second loop publishes
// from that envelope at wall-clock speed, starting when the player starts. What
// reaches the interface is then the amplitude of the audio at the moment it is
// being heard.
//
// One inaccuracy remains and cannot be removed from here: the player keeps its
// own buffer, so the sound leaves the speakers something under a tenth of a
// second after we say it does. That is small enough not to see.
type voiceMeter struct {
	rate  int
	block int // samples per envelope entry

	mu     sync.Mutex
	rest   []byte
	blocks []float64

	started time.Time
	// sealed is set once the synthesiser has produced everything it is going
	// to. Until then, running past the end of the envelope means generation is
	// behind rather than that the sound has ended.
	sealed bool

	stop chan struct{}
	done chan struct{}
}

// blockDuration is the resolution of the envelope. Short enough to follow the
// shape of speech, long enough that the display is not jittering on individual
// glottal pulses.
const blockDuration = 20 * time.Millisecond

// publishInterval is how often the level is refreshed. The interface polls at a
// similar rate; going finer would only produce readings nobody reads.
const publishInterval = 40 * time.Millisecond

func newVoiceMeter(rate int) *voiceMeter {
	if rate <= 0 {
		rate = 22050
	}

	return &voiceMeter{
		rate:  rate,
		block: int(float64(rate) * blockDuration.Seconds()),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// Write records audio on its way to the player. It never fails and never
// blocks the pipe: if measurement went wrong the sound must still play.
func (m *voiceMeter) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rest = append(m.rest, p...)

	// Samples are 16-bit, so a sample can be split across two writes. The
	// remainder is carried rather than dropped, otherwise the envelope would
	// drift out of step with the sound over a long answer.
	for len(m.rest) >= m.block*2 {
		var sum float64

		for i := 0; i < m.block; i++ {
			s := int16(binary.LittleEndian.Uint16(m.rest[i*2:]))
			sum += float64(s) * float64(s)
		}

		m.blocks = append(m.blocks, math.Sqrt(sum/float64(m.block)))
		m.rest = m.rest[m.block*2:]
	}

	return len(p), nil
}

// start begins publishing, and should be called when the player does.
func (m *voiceMeter) start() {
	m.mu.Lock()
	m.started = time.Now()
	m.mu.Unlock()

	go m.run()
}

func (m *voiceMeter) run() {
	defer close(m.done)

	ticker := time.NewTicker(publishInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
		}

		publishLevel("voice", m.at(time.Since(m.started)))
	}
}

// at reports the recorded amplitude of the sound at a moment in the playback.
//
// Wall-clock time is the playback position only while the synthesiser is
// keeping ahead of the player, and on a machine already running a language
// model it often is not: measured here, piper produces about 1.3 seconds of
// speech per second of work, and less than that under load. When it falls
// behind, the player runs out of audio and waits — playback pauses, wall-clock
// time does not, and the clock runs off the end of the envelope.
//
// Reporting silence at that point would be precisely wrong: the display would
// go flat in the middle of a sentence, which is the one thing this meter exists
// to avoid. So running past the end means different things depending on whether
// the sound has all been made yet. Before it has, the newest block stands,
// because that is genuinely the most recent thing measured. After it has, the
// sound really is over.
func (m *voiceMeter) at(elapsed time.Duration) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.blocks) == 0 {
		return 0
	}

	i := int(elapsed / blockDuration)

	if i < 0 {
		return 0
	}

	if i >= len(m.blocks) {
		if m.sealed {
			return 0
		}

		return m.blocks[len(m.blocks)-1]
	}

	return m.blocks[i]
}

// seal records that the synthesiser has finished producing sound.
func (m *voiceMeter) seal() {
	m.mu.Lock()
	m.sealed = true
	m.mu.Unlock()
}

// finish stops publishing and leaves the meter silent.
func (m *voiceMeter) finish() {
	close(m.stop)
	<-m.done

	clearLevel("voice")
}
