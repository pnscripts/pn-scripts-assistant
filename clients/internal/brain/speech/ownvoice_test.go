package speech

import (
	"os"
	"strings"
	"testing"
	"time"
)

/*
 * The loop that ran for three rounds in a minute.
 *
 * It asked "I heard siga.joshina.com but I do not know that name — is that
 * right?", heard its own question through the microphone, found a name inside
 * it that it also did not know, and asked again about that one. Every round
 * invented the next name and every question was addressed to nobody. What kept
 * it alive was its own name, which appears in almost everything it says, so
 * the wake word matched each time.
 *
 * The check is on the words rather than on the sound, and that is the point:
 * echo cancellation only has to fail slightly to fail completely, and it fails
 * worst exactly where this matters — a microphone sitting next to the speaker.
 */
func TestItDoesNotAnswerItsOwnVoice(t *testing.T) {
	ForgetSpokenWords()
	defer ForgetSpokenWords()

	JustSaid("I heard siga.joshina.com but I do not know that name — is that " +
		"right? Say it again or type it, and I will remember the spelling.")

	// Exactly as it came back through the microphone: most of it, misheard in
	// places, and cut off where the turn ended.
	for _, echo := range []string{
		"I heard siga.joshina.com but I do not know that name. Is that right?",
		"I heard breenit.dancie.anyupcens.com, but I did not know that name. Is",
		"but I do not know that name — say it again or type it and I will remember",
	} {
		if !SoundsLikeItself(echo) {
			t.Errorf("its own voice was accepted as a turn: %q", echo)
		}
	}
}

/*
 * Somebody actually talking is never mistaken for the echo.
 *
 * The more dangerous failure of the two. An assistant that occasionally
 * answers itself is annoying; one that ignores its owner because they used
 * words it had used first is broken, and the fault is invisible — from the
 * outside it looks exactly like the microphone having stopped working.
 */
func TestARealAnswerIsNeverMistakenForTheEcho(t *testing.T) {
	ForgetSpokenWords()
	defer ForgetSpokenWords()

	JustSaid("I heard siga.joshina.com but I do not know that name — is that " +
		"right? Say it again or type it, and I will remember the spelling.")

	for _, real := range []string{
		"no that is not right",
		"it is pnscripts.com",
		"Brain, what is the weather in Sofia today?",
		"yes that is right, remember it",
		// Uses some of the same words, and is plainly a person replying.
		"that name is wrong, the spelling is pnscripts",
	} {
		if SoundsLikeItself(real) {
			t.Errorf("a real answer was discarded as the assistant's own voice: %q", real)
		}
	}
}

// Nothing said means nothing to compare against, and everything gets through.
func TestWithNothingSaidEverythingIsHeard(t *testing.T) {
	ForgetSpokenWords()

	if SoundsLikeItself("I heard siga.joshina.com but I do not know that name") {
		t.Error("a transcript was discarded when the assistant had said nothing")
	}
}

/*
 * Names are not taken from near-silence.
 *
 * Whisper does not decline. Given a second of almost nothing it returns
 * something, and what it returns often has the shape of a name:
 * "siga.joshina.com" and "breenit.dancie.anyupcens.com" came from audio
 * peaking at 270 and 411, while real speech in the same room peaked between
 * 3200 and 6700. Asking about those teaches nothing, interrupts, and gives the
 * microphone another sentence to mishear.
 */
func TestNamesAreNotInventedFromSilence(t *testing.T) {
	SetVocabulary(func() []string { return nil })
	defer SetVocabulary(nil)

	const invented = "I heard breenit.dancie.anyupcens.com"

	if got := UnfamiliarIn(invented, 411); len(got) > 0 {
		t.Errorf("asked about a name heard at peak 411: %v", got)
	}

	// The same words, said properly, are worth asking about.
	if got := UnfamiliarIn(invented, 3258); len(got) == 0 {
		t.Error("a name said clearly at peak 3258 was not queried")
	}
}

/*
 * A room where the microphone sits next to the speaker gives up on listening
 * while it talks, rather than going on talking to itself.
 *
 * Catching an echo in the transcript still costs a whole turn — the microphone
 * was occupied, the recogniser ran, and whoever was waiting was not being
 * heard. A few of those in a row is not bad luck, it is the arrangement of the
 * furniture, and the useful response is to stop trying until it has finished
 * speaking.
 */
func TestALiveRoomStopsListeningWhileItSpeaks(t *testing.T) {
	ForgetEchoes()
	defer ForgetEchoes()

	if RoomIsTooLive() {
		t.Fatal("a room gave up before hearing a single echo")
	}

	// One is a fluke: a door, a loud passage, a moment of distortion.
	HeardItself()

	if RoomIsTooLive() {
		t.Error("one echo was treated as a bad room")
	}

	for i := 1; i < EchoesBeforeGivingUp; i++ {
		HeardItself()
	}

	if !RoomIsTooLive() {
		t.Errorf("%d echoes did not count as a room where this cannot work",
			EchoesBeforeGivingUp)
	}
}

/*
 * Whatever voice says it, the brain must recognise it coming back.
 *
 * Recording what was said lived inside the piper path alone. The robot voice
 * goes through speech-dispatcher, so when the robot became the default the
 * brain stopped recognising itself — and transcribed its own greeting as
 * something somebody had said to it, wake word and all.
 */
func TestEverySpeechPathRemembersWhatItSaid(t *testing.T) {
	source, err := os.ReadFile("speech.go")
	if err != nil {
		t.Fatal(err)
	}

	s := string(source)

	// Both entry points make sound, so both must remember.
	for _, fn := range []string{
		"func SpeakAndWait(ctx context.Context, text string) error {",
		"func Speak(ctx context.Context, text string) error {",
	} {
		at := strings.Index(s, fn)
		if at < 0 {
			t.Errorf("%s has moved; this test is out of date", fn)

			continue
		}

		// Within the function, before it dispatches to any engine.
		body := s[at:]
		if end := strings.Index(body, "\nfunc "); end > 0 {
			body = body[:end]
		}

		if !strings.Contains(body, "JustSaid(") {
			t.Errorf("%s makes sound without remembering it", strings.TrimSuffix(fn, " {"))
		}
	}
}

/*
 * The echo that arrives mixed with something else.
 *
 * This is a regression test for a brain answering itself in a loop. Music was
 * playing, the assistant spoke, and what came back through the microphone was
 * a blend: part its own sentence, part the music, part nothing. Judged on
 * overlap the blend looked like a stranger — most of the words in it were not
 * the assistant's — so it was answered, which produced more speech, which
 * produced another blend.
 */
func TestAnEchoMixedWithOtherSoundIsStillItsOwnVoice(t *testing.T) {
	ForgetSpokenWords()

	JustSaid("One more check of the level while talking.")

	// Exactly what the microphone returned, transcribed.
	heard := "play, the music. One more check of the level, quail talking. What?"

	if !SoundsLikeItself(heard) {
		t.Fatalf("answered its own voice mixed with music:\n%q", heard)
	}
}

/*
 * And the failure that must not come back with it.
 *
 * Somebody answering an assistant reuses its words constantly, because they
 * are answering it. Discarding those is much the worse of the two failures: a
 * brain that occasionally answers itself is irritating, one that ignores its
 * owner is broken.
 */
func TestSomebodyAnsweringInTheAssistantsOwnWordsIsStillHeard(t *testing.T) {
	ForgetSpokenWords()

	JustSaid("I can change the file in that folder, or leave it as it is. Which would you like?")

	for _, said := range []string{
		"yes, change the file",
		"no, leave it as it is",
		"what folder did you mean by that one",
		"do it in the other project instead",
		"actually leave the file and tell me what is in it",
	} {
		if SoundsLikeItself(said) {
			t.Errorf("ignored its owner saying %q", said)
		}
	}
}

/*
 * A reply that is almost nothing but the assistant's own words is discarded,
 * and that is the older rule rather than the run.
 *
 * Recorded here because it is a real cost and a deliberate one: "change the
 * file in that folder please" is six words of which five are the assistant's,
 * and the bag test cannot tell that from an echo. The run test would let it
 * through — five words is under half of what was said — so if the bag rule is
 * ever loosened, this is the case to think about.
 */
func TestANearVerbatimReplyIsDiscardedByTheOlderRule(t *testing.T) {
	ForgetSpokenWords()

	JustSaid("I can change the file in that folder, or leave it as it is. Which would you like?")

	if !SoundsLikeItself("change the file in that folder please") {
		t.Skip("the bag rule has changed; check whether this is now answered")
	}

	// And the run test on its own would not have done this.
	if sharesARunWithSomethingSaid("change the file in that folder please") {
		t.Error("the run test also caught it, which was not the intention")
	}
}

// Six words in a row is the signal; four is a person agreeing.
func TestARunOfWordsIsWhatCounts(t *testing.T) {
	ForgetSpokenWords()

	JustSaid("The operations panel now shows how long answering takes on this machine.")

	if SoundsLikeItself("the operations panel, yes") {
		t.Error("four words of agreement were treated as an echo")
	}

	if !SoundsLikeItself("something something the operations panel now shows how long, and then nothing") {
		t.Error("seven consecutive words of its own were not recognised")
	}
}

// An echo that has aged out is not one: the words may genuinely be said again.
func TestAnOldSentenceIsNotAnEchoForever(t *testing.T) {
	ForgetSpokenWords()

	JustSaid("The operations panel now shows how long answering takes.")

	spoken.mu.Lock()
	for i := range spoken.lines {
		spoken.lines[i].at = time.Now().Add(-RememberSpeechFor - time.Second)
	}
	spoken.mu.Unlock()

	if SoundsLikeItself("the operations panel now shows how long answering takes") {
		t.Error("a sentence from a minute ago was still treated as an echo")
	}
}

func TestTheLongestRunIsFoundWhereverItSits(t *testing.T) {
	for _, c := range []struct {
		a, b []string
		want int
	}{
		{[]string{"one", "two", "three"}, []string{"one", "two", "three"}, 3},
		{[]string{"x", "one", "two", "three", "y"}, []string{"one", "two", "three"}, 3},
		{[]string{"one", "x", "two"}, []string{"one", "two"}, 1},
		{[]string{}, []string{"one"}, 0},
		{[]string{"one"}, nil, 0},
		{[]string{"a", "b", "c"}, []string{"d", "e"}, 0},
	} {
		if got := longestRun(c.a, c.b); got != c.want {
			t.Errorf("%v against %v: %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// Both sides are filtered the same way, or every run breaks at the first
// two-letter word — which is every other word in English.
func TestBothSidesAreFilteredAlike(t *testing.T) {
	said := wordsOf("One more check of the level while talking")
	bag := bagOf("One more check of the level while talking")

	for _, word := range said {
		if !bag[word] {
			t.Errorf("%q survived one filter and not the other", word)
		}
	}

	if len(said) != len(bag) {
		t.Errorf("the ordered list has %d words and the bag %d", len(said), len(bag))
	}
}
