package speech

import (
	"strings"
	"sync"
	"time"
)

/*
 * Refusing to answer itself.
 *
 * The echo canceller removes most of the assistant's voice from the
 * microphone, and most is not enough. What leaks through is still speech, the
 * recogniser still transcribes it, and the wake word is still in it — so the
 * brain hears itself, decides it was addressed, and answers. Then it hears
 * that answer too.
 *
 * Observed, not imagined. It asked "I heard siga.joshina.com but I do not know
 * that name — is that right?", transcribed its own question back, found a name
 * in it that it also did not know, and asked again about that. Three rounds in
 * a minute, each one inventing the next name, and every one of them addressed
 * to nobody. The word that kept the loop alive was its own name, which appears
 * in almost everything it says.
 *
 * Cancellation cannot fix this on its own, because it only has to fail
 * slightly to fail completely — one leaked sentence is one whole turn. So what
 * it said is remembered, briefly, and anything that comes back sounding like
 * it is discarded before it can become a turn. That check does not depend on
 * the acoustics being good, which is the point: it holds on a laptop with the
 * speakers next to the microphone, and it holds when somebody turns the volume
 * up.
 */

const (
	// RememberSpeechFor is how long its own words stay comparable.
	//
	// Long enough to cover the sentence being played plus the recogniser's
	// delay behind it, and short enough that somebody deliberately repeating
	// the brain's own phrasing a minute later is still heard.
	RememberSpeechFor = 25 * time.Second

	/*
	 * SameEnough is the share of words that must match to count as an echo.
	 *
	 * Not an exact comparison, because what comes back is never exact: the
	 * canceller removes some of it, the recogniser mishears part of what is
	 * left, and the tail is usually cut off when the turn ends.
	 */
	SameEnough = 0.66

	/*
	 * LongEnoughToJudge is the shortest transcript worth comparing at all.
	 *
	 * Short replies are the ones a person actually gives — "no that is not
	 * right", "yes, remember it" — and they are built almost entirely from
	 * words the assistant just used, because they are answering it. Judged on
	 * overlap they look exactly like an echo, and discarding them is the worse
	 * failure of the two by a long way: an assistant that occasionally answers
	 * itself is irritating, one that ignores its owner is broken, and from the
	 * outside that looks like the microphone having died.
	 */
	LongEnoughToJudge = 6

	/*
	 * EnoughSharedWords stops a short answer matching on common words alone.
	 *
	 * "That name is wrong, the spelling is pnscripts" shares four words with a
	 * question about a name and a spelling, which is two thirds of it — enough
	 * to pass the ratio and be thrown away. A real echo of a whole sentence
	 * shares far more than four.
	 */
	EnoughSharedWords = 5
)

var spoken = struct {
	mu    sync.RWMutex
	lines []spokenLine
}{}

type spokenLine struct {
	words map[string]bool
	count int
	at    time.Time
}

// JustSaid records something the assistant has said aloud.
func JustSaid(text string) {
	words := bagOf(text)
	if len(words) < 3 {
		// Too short to compare on. "Yes." said back is somebody agreeing.
		return
	}

	spoken.mu.Lock()
	defer spoken.mu.Unlock()

	spoken.lines = append(spoken.lines, spokenLine{
		words: words,
		count: len(words),
		at:    time.Now(),
	})

	// Only the recent past is worth keeping.
	fresh := spoken.lines[:0]

	for _, line := range spoken.lines {
		if time.Since(line.at) < RememberSpeechFor {
			fresh = append(fresh, line)
		}
	}

	spoken.lines = fresh
}

/*
 * SoundsLikeItself reports that a transcript is the assistant's own voice.
 *
 * Compared as a bag of words rather than as a string, because what returns
 * through a microphone is a damaged copy: words dropped, others misheard, the
 * ending cut where the turn stopped. Word order survives none of that reliably
 * and the vocabulary mostly does.
 */
func SoundsLikeItself(text string) bool {
	heard := bagOf(text)

	// Short answers are always let through; see LongEnoughToJudge.
	if len(heard) < LongEnoughToJudge {
		return false
	}

	spoken.mu.RLock()
	defer spoken.mu.RUnlock()

	for _, line := range spoken.lines {
		if time.Since(line.at) >= RememberSpeechFor {
			continue
		}

		var shared int

		for word := range heard {
			if line.words[word] {
				shared++
			}
		}

		if shared < EnoughSharedWords {
			continue
		}

		/*
		 * Measured against what was heard, not against what was said.
		 *
		 * The turn usually ends before the assistant has finished talking, so
		 * a genuine echo is a fragment of a longer sentence. Scoring it
		 * against the whole spoken line would make every echo look barely
		 * similar; scoring it against the fragment asks the right question —
		 * is nearly everything in this transcript something I just said?
		 */
		if float64(shared)/float64(len(heard)) >= SameEnough {
			return true
		}
	}

	return false
}

// ForgetSpokenWords clears the record, for tests and for a fresh conversation.
func ForgetSpokenWords() {
	spoken.mu.Lock()
	spoken.lines = nil
	spoken.mu.Unlock()
}

// bagOf reduces a sentence to the distinct words worth comparing.
func bagOf(text string) map[string]bool {
	out := map[string]bool{}

	for _, word := range strings.Fields(strings.ToLower(text)) {
		word = strings.Trim(word, `.,!?;:"'()[]…—-`)

		// Very short words are in every sentence and distinguish nothing.
		if len(word) > 2 {
			out[word] = true
		}
	}

	return out
}

/*
 * Noticing a room where the microphone is next to the speaker.
 *
 * Echo cancellation assumes the microphone hears a quieter, delayed copy of
 * what the speaker played. Put the two a hand's width apart and that stops
 * being true: the copy arrives loud enough to clip, distorted in ways the
 * canceller cannot model, and enough of it survives to be transcribed as
 * speech every time the assistant opens its mouth.
 *
 * The text check catches those, which is why it exists. But catching an echo
 * still costs a whole turn — the microphone was occupied, the recogniser ran,
 * and the person waiting was not being listened to — so a room that produces
 * them repeatedly is a room to change behaviour in rather than keep coping
 * with.
 *
 * The change is to stop listening while speaking, which costs the ability to
 * interrupt by voice and keeps everything else working. That is the right way
 * round: the Stop button and Escape still interrupt, and an assistant that
 * cannot be interrupted mid-sentence is far better than one that spends every
 * sentence talking to itself.
 */

// EchoesBeforeGivingUp is how many self-heard turns count as a bad room.
//
// Three, because one is a fluke — a door, a loud passage, a moment of
// distortion — and three inside a couple of minutes is the arrangement of the
// furniture.
const EchoesBeforeGivingUp = 3

// EchoMemory is how long those count for.
const EchoMemory = 3 * time.Minute

var echoes = struct {
	mu   sync.Mutex
	when []time.Time
}{}

// HeardItself records that a turn turned out to be the assistant's own voice.
func HeardItself() {
	echoes.mu.Lock()
	defer echoes.mu.Unlock()

	now := time.Now()

	fresh := echoes.when[:0]

	for _, at := range echoes.when {
		if now.Sub(at) < EchoMemory {
			fresh = append(fresh, at)
		}
	}

	echoes.when = append(fresh, now)
}

/*
 * RoomIsTooLive reports that listening while speaking is not working here.
 *
 * Consulted by the recorder, which then stays quiet until the assistant has
 * stopped talking. Recovers by itself: the count ages out, so moving the
 * microphone or turning the volume down brings interrupting back without
 * anybody having to find a setting.
 */
func RoomIsTooLive() bool {
	echoes.mu.Lock()
	defer echoes.mu.Unlock()

	var recent int

	for _, at := range echoes.when {
		if time.Since(at) < EchoMemory {
			recent++
		}
	}

	return recent >= EchoesBeforeGivingUp
}

// ForgetEchoes clears the count, for tests.
func ForgetEchoes() {
	echoes.mu.Lock()
	echoes.when = nil
	echoes.mu.Unlock()
}
