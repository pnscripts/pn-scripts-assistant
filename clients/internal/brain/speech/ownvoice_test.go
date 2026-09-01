package speech

import (
	"os"
	"strings"
	"testing"
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
