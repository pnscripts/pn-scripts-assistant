package speech

import (
	"os"
	"path/filepath"
	"strings"
)

/*
 * Silero, deciding whether anybody actually spoke.
 *
 * The level detector answers a different question from the one that matters.
 * It measures loudness, and loudness is not speech: a fan, a chair, a lorry
 * outside and a hand on the desk all cross any threshold set low enough to
 * catch a person talking quietly. Every one of those became a turn — recorded,
 * transcribed, and answered.
 *
 * Answered, because the recogniser does not decline. Given four seconds of
 * room noise it returned "(crickets chirping)", and given near-silence it
 * returns a hopeful fragment of whatever it has heard most often. Measured
 * here rather than assumed: the same four seconds through Silero come back
 * empty, as they should.
 *
 * This is not a new dependency. whisper.cpp has carried VAD since well before
 * the build on this machine, so it is a flag and a model file rather than
 * another program to install — which matters, because everything here has to
 * work for somebody who has never opened a terminal.
 */

// vadModelNames are the VAD models whisper.cpp ships, newest first.
//
// The for-tests one is deliberately absent. It is a real Silero model and it
// would work, but it is there to make the test suite deterministic and there
// is no promise it stays.
var vadModelNames = []string{
	"ggml-silero-v5.1.2.bin",
	"ggml-silero-v5.1.bin",
	"ggml-silero.bin",
}

/*
 * FindVADModel returns the speech detector's weights, or "" if not fetched.
 *
 * Absence is not an error anywhere it is used. Without it the level detector
 * is all there is, which is how this program ran until now — worse, but not
 * broken, and a machine that has not downloaded the file yet should still
 * answer when spoken to.
 */
func FindVADModel() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	for _, dir := range modelSearch(home) {
		for _, name := range vadModelNames {
			path := filepath.Join(dir, name)

			// About 900KB. A truncated download is worse than none, because it
			// fails inside whisper rather than here, where the message would
			// have said which file was wrong.
			if info, err := os.Stat(path); err == nil && info.Size() > 400<<10 {
				return path
			}
		}
	}

	return ""
}

/*
 * withVAD adds speech detection to a whisper command line.
 *
 * The thresholds are deliberately more forgiving than Silero's defaults. This
 * runs after something has already decided a turn is worth transcribing, so
 * its job is to throw out what is plainly not speech, not to adjudicate
 * whether a quiet sentence was quiet enough to count. Discarding real words
 * because somebody spoke softly is a far worse failure than passing a little
 * noise through to the recogniser, and it is the failure this program has
 * spent two days producing by other means.
 */
func withVAD(args []string) []string {
	model := FindVADModel()
	if model == "" {
		return args
	}

	return append(args,
		"--vad",
		"-vm", model,
		// Below Silero's 0.5 default: a hesitant or distant speaker still counts.
		"-vt", "0.35",
		// A word like "yes" or "stop" is shorter than the 250ms default, and
		// both are exactly the kind of thing said to interrupt.
		"-vspd", "150",
		// Keeps the breath before a sentence and the tail of the last word,
		// both of which the recogniser uses and neither of which is speech.
		"-vp", "60",
	)
}

// vadReady reports whether speech detection is available, for the interface to
// show honestly rather than implying a check that is not happening.
func vadReady() bool {
	return FindVADModel() != ""
}

// looksLikeNoise reports whether a transcript is the recogniser guessing.
//
// A second line of defence, and needed even with Silero: whisper narrates what
// it cannot transcribe, in brackets, and those descriptions are not speech and
// must never become a turn. "(crickets chirping)" was a real answer to a real
// recording of an empty room.
func looksLikeNoise(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}

	/*
	 * Bracketed throughout, in any of the three kinds whisper uses.
	 *
	 * Only when the whole line is bracketed. A sentence that happens to
	 * contain an aside is somebody talking, and dropping it would lose the
	 * turn.
	 */
	for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}, {"*", "*"}} {
		if strings.HasPrefix(trimmed, pair[0]) && strings.HasSuffix(trimmed, pair[1]) {
			return true
		}
	}

	return false
}
