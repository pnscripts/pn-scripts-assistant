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
	 * EchoRun is how many words in a row make a transcript its own voice
	 * whatever else is mixed in with it.
	 *
	 * Five, which people do not reach by accident. Somebody answering reuses
	 * an assistant's words constantly — "yes, change the file", "no, the other
	 * folder" — and three or four in a row happens; six in the same order does
	 * not, unless it came back through a microphone.
	 *
	 * Getting this wrong in the tight direction ignores its owner, which is
	 * the worse failure of the two. Five is deliberately conservative.
	 */
	EchoRun = 5

	/*
	 * MostOfWhatWasSaid is how much of a spoken line a run has to cover.
	 *
	 * Half. A person quoting an assistant back at itself takes a phrase out of
	 * a sentence; a microphone hands back the sentence with pieces missing. It
	 * is the proportion rather than the length that tells them apart.
	 */
	MostOfWhatWasSaid = 0.5

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

	/*
	 * order is the same words in the order they were said.
	 *
	 * Kept as well as the bag, for the case the bag cannot see: a transcript
	 * that is part its own voice and part something else. The overlap is then
	 * measured against a mixture and comes out low, so the echo passes — while
	 * a run of six consecutive words it had just said sits in the middle of
	 * it, which is not something that happens by chance.
	 *
	 * That is not a hypothetical. "play, the music. One more check of the
	 * level, quail talking. What?" was answered as a question from somebody,
	 * and six of those words were said by the assistant four seconds earlier.
	 */
	order []string

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
		order: wordsOf(text),
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

	return sharesARunWithSomethingSaid(text)
}

/*
 * sharesARunWithSomethingSaid catches an echo mixed with something else.
 *
 * The bag comparison above asks "is nearly everything in this transcript
 * something I just said", which is the right question for a clean echo and the
 * wrong one for a dirty one. Music playing over the top, or two voices in the
 * room, and the transcript comes back as a blend: half the assistant's
 * sentence, half something else, and an overlap ratio too low to act on. That
 * blend is what produced a brain answering itself in a loop.
 *
 * So: a run of consecutive words. People answering an assistant reuse its
 * words constantly — "yes, change the file" — but they do not reproduce six of
 * them in a row in the order it said them. A microphone does.
 */
func sharesARunWithSomethingSaid(text string) bool {
	heard := wordsOf(text)

	if len(heard) < EchoRun {
		return false
	}

	spoken.mu.RLock()
	defer spoken.mu.RUnlock()

	for _, line := range spoken.lines {
		if time.Since(line.at) >= RememberSpeechFor {
			continue
		}

		run := longestRun(heard, line.order)

		if run < EchoRun || len(line.order) == 0 {
			continue
		}

		/*
		 * And the run has to be most of what was said, not a phrase out of it.
		 *
		 * This is what separates the two cases, and a length alone cannot.
		 * Somebody answering quotes a fragment of a long sentence back —
		 * "change the file in that folder please", five words of a nineteen
		 * word offer — while a microphone returns most of a short one. The
		 * first must be answered and the second must not, and both are five
		 * words in a row.
		 */
		if float64(run)/float64(len(line.order)) >= MostOfWhatWasSaid {
			return true
		}
	}

	return false
}

/*
 * longestRun is the longest run of words appearing in both, in order.
 *
 * The textbook table, which is more than this needs and less than it costs to
 * think about: both sides are one sentence, so this is at worst a few hundred
 * comparisons on a transcript that took a second of processor to produce.
 */
func longestRun(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)

	best := 0

	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				current[j] = previous[j-1] + 1

				if current[j] > best {
					best = current[j]
				}
			} else {
				current[j] = 0
			}
		}

		previous, current = current, previous
	}

	return best
}

/*
 * wordsOf is the same words bagOf keeps, in the order they were said.
 *
 * The same filtering on purpose. Comparing a filtered sentence against an
 * unfiltered one would break every run at the first "of" or "to", which is
 * every other word in English.
 */
func wordsOf(text string) []string {
	var out []string

	for _, word := range strings.Fields(strings.ToLower(text)) {
		word = strings.Trim(word, `.,!?;:"'()[]…—-`)

		if len(word) > 2 {
			out = append(out, word)
		}
	}

	return out
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
