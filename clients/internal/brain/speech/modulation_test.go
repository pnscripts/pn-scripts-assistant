package speech

import "testing"

/*
 * The cooling fan does not start a turn; a voice does.
 *
 * Diagnosed from a recording rather than guessed at. A failing turn was kept
 * and held twenty-three seconds at an RMS between 3900 and 5200 in every
 * single second, with no gaps anywhere. Whisper, asked with no speech
 * detection at all, described it as "(engine revving)" — the microphone sits
 * on a desk beside a machine whose processor is pinned while the model runs,
 * and it was hearing the fan.
 *
 * Loudness cannot tell those apart, because broadband noise peaks well above
 * its own average and that is all a threshold asks. Speech is made of
 * syllables and swings far wider.
 */
func TestASteadyFanIsNotMistakenForAVoice(t *testing.T) {
	// A fan, as measured: loud, and the same from one frame to the next.
	fan := []int{4957, 4849, 4119, 4051, 4150, 4505, 5017, 5003, 4588, 4392}

	if soundsLikeAVoice(fan) {
		t.Error("steady machine noise was accepted as somebody talking, which is " +
			"how turns full of fan noise came to be recorded")
	}

	// Speech, which rises and falls between syllables and word gaps.
	voice := []int{180, 2400, 3100, 900, 260, 2800, 3400, 1100, 240, 2900}

	if !soundsLikeAVoice(voice) {
		t.Error("a voice was rejected, which means being ignored — the worse fault")
	}

	// Somebody speaking quietly still swings, just at a lower level.
	quiet := []int{60, 420, 610, 180, 70, 500, 640, 210, 80, 470}

	if !soundsLikeAVoice(quiet) {
		t.Error("a quiet voice was rejected")
	}

	// Too little history to judge fails open, because being ignored is worse
	// than a wasted turn.
	if !soundsLikeAVoice([]int{4000, 4000}) {
		t.Error("a short window was rejected rather than given the benefit of doubt")
	}
}

/*
 * The fan, as it was actually measured, over a whole turn.
 *
 * Written from the recordings kept when turns failed rather than from a guess
 * at what a fan looks like. Across eighty-eight windows of a forty-five second
 * recording that whisper described as "(engine revving)", either none or one
 * swung like speech. Real speech through the same room swung in eighty-nine to
 * a hundred per cent of its windows.
 *
 * The earlier version of this test read whichever file happened to be in the
 * kept-recordings folder, which made it depend on what had gone wrong most
 * recently — so it passed or failed according to the weather. The numbers it
 * was measuring are written down here instead.
 */
func TestAWholeTurnOfFanNoiseIsRejected(t *testing.T) {
	// Eighty-eight windows of steady noise, as measured: loud, and level.
	var fan []int

	for i := 0; i < 450; i++ {
		fan = append(fan, 3900+(i*37)%1300)
	}

	if wholeTurnSoundsLikeAVoice(fan) {
		t.Error("forty-five seconds of steady noise still reads as a voice, " +
			"which is how a recording of a cooling fan reached the recogniser")
	}

	// One waver in the middle is not speech either: that single window is what
	// the first version of this check accepted.
	fan[220], fan[221], fan[222] = 400, 6000, 400

	if wholeTurnSoundsLikeAVoice(fan) {
		t.Error("one swinging window in eighty-eight was enough, which is the " +
			"bug this replaced")
	}

	// Speech: rising and falling throughout, syllable by syllable.
	var voice []int

	for i := 0; i < 60; i++ {
		if i%3 == 0 {
			voice = append(voice, 180)
		} else {
			voice = append(voice, 2600)
		}
	}

	if !wholeTurnSoundsLikeAVoice(voice) {
		t.Error("speech was rejected, which means being ignored — the worse fault")
	}

	// And somebody who says one sentence then lets the fan run has still
	// spoken. Losing that is worse than transcribing a little noise.
	spoke := append(append([]int{}, voice...), fan[:200]...)

	if !wholeTurnSoundsLikeAVoice(spoke) {
		t.Error("a sentence followed by silence and fan noise was thrown away")
	}
}
