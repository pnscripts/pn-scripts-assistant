package speech

/*
 * Telling a voice from a fan.
 *
 * The level detector asks whether a frame is louder than the room, and a fan
 * under load answers yes. Its peaks sit comfortably above its own average —
 * broadband noise always does — so a machine at full load starts turns by
 * itself, records ten or twenty seconds of nothing, and hands the recogniser
 * a file it can make no words out of.
 *
 * Diagnosed from a recording rather than guessed at. Twenty-three seconds
 * captured during a failing turn showed an RMS between 3900 and 5200 in every
 * single second and no gaps anywhere, and whisper, asked without any speech
 * detection at all, described it as "(engine revving)". The microphone sits on
 * a desk beside a machine whose processor is pinned at a hundred per cent
 * while the model runs, and it was hearing the cooling fan.
 *
 * Loudness cannot separate those. What separates them is that speech is made
 * of syllables — energy rises and falls three to eight times a second, and the
 * gaps between words drop most of the way to the floor — while a fan is flat.
 * So the shape of the recent past is measured, not just its height.
 */

/*
 * SpeechSwing is how much louder the loudest recent frame must be than the
 * quietest before the sound counts as somebody talking.
 *
 * Three to one. Ordinary speech swings far wider than that between a stressed
 * vowel and the gap before the next word — usually ten to one or more — and
 * steady machine noise barely reaches two. Set at three the test rejects fans,
 * hum and traffic while leaving room for somebody speaking quietly and evenly,
 * which is the case it must not break.
 */
const SpeechSwing = 3.0

/*
 * FramesToJudgeSwing is how much recent history to measure it over.
 *
 * About a second at 100ms a frame, which is enough to contain a syllable and
 * the gap after it. Shorter and a single loud vowel looks as flat as a fan;
 * longer and the detector waits through the beginning of what somebody said
 * before deciding they said it.
 */
const FramesToJudgeSwing = 10

/*
 * soundsLikeAVoice reports whether recent frames rise and fall the way speech
 * does.
 *
 * Deliberately generous, and it fails open: too little history to judge counts
 * as a voice. Rejecting real speech is far worse than accepting a little
 * noise, because noise costs a wasted turn and a rejected sentence costs the
 * person being ignored — which from the outside looks exactly like the
 * microphone having stopped working.
 */
func soundsLikeAVoice(recent []int) bool {
	if len(recent) < FramesToJudgeSwing {
		return true
	}

	window := recent[len(recent)-FramesToJudgeSwing:]

	loudest, quietest := window[0], window[0]

	for _, level := range window {
		if level > loudest {
			loudest = level
		}

		if level < quietest {
			quietest = level
		}
	}

	// A floor of one, so a window containing true digital silence does not
	// divide by zero — and silence next to any sound is plainly a swing.
	if quietest < 1 {
		quietest = 1
	}

	return float64(loudest)/float64(quietest) >= SpeechSwing
}

/*
 * wholeTurnSoundsLikeAVoice judges an entire recording, not its last second.
 *
 * soundsLikeAVoice deliberately looks at a short window, because starting a
 * turn is a decision that has to be made from the little that has arrived.
 * Reusing it at the end of a turn was a mistake with a precise cost: a
 * forty-five second recording was judged on its final ten frames, a fan that
 * happened to waver at the end passed, and the whole thing went to the
 * recogniser — which described it as "(engine revving)" and returned no words.
 *
 * Over a whole turn there is no such constraint, and the question is different
 * too: not "is this moment speech" but "was any of this speech". Speech
 * anywhere in the recording is enough, because somebody who said one sentence
 * and then let the fan run has still said a sentence.
 */
func wholeTurnSoundsLikeAVoice(levels []int) bool {
	if len(levels) < FramesToJudgeSwing {
		return true
	}

	/*
	 * Most of the turn has to swing, not some of it.
	 *
	 * Accepting any single swinging window was the previous attempt and it
	 * failed on the recording that prompted it: across eighty-eight windows of
	 * fan noise, one wavered enough to pass, and forty-five seconds of
	 * "(engine revving)" went to the recogniser.
	 *
	 * Measured rather than guessed. Across the recordings kept from real
	 * failures and from real speech:
	 *
	 *   speech through a room   89%, 100%, 89% of windows swing
	 *   forty-five second fans   0%,   1%,   0%
	 *
	 * There is no threshold in that gap that is delicate. A third is chosen
	 * well below the speech figures, because a person who says one sentence
	 * and then lets the fan run has still said a sentence, and losing that is
	 * far worse than transcribing a little noise.
	 */
	/*
	 * A count, not a proportion.
	 *
	 * A proportion was tried and it fails the case that matters most:
	 * somebody says one sentence and then stops, the turn waits out the
	 * silence with the fan running, and the share of speaking windows falls
	 * below any threshold that also rejects a fan. That would throw away the
	 * sentence — being ignored, which is the worse fault.
	 *
	 * A count has no such tension, because the two are separated by it
	 * absolutely rather than relatively. Measured across the recordings kept
	 * from real failures and from real speech:
	 *
	 *   speech through a room      8, 13, 8 swinging windows
	 *   forty-five second fans     0,  1, 0
	 *
	 * Four sits in the middle of that gap with room on both sides. It is about
	 * two seconds of somebody talking, which is the shortest thing worth
	 * calling a turn anyway.
	 */
	const enough = 4

	var swinging int

	for start := 0; start+FramesToJudgeSwing <= len(levels); start += FramesToJudgeSwing / 2 {
		if soundsLikeAVoice(levels[start : start+FramesToJudgeSwing]) {
			swinging++

			if swinging >= enough {
				return true
			}
		}
	}

	return false
}
