package speech

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

// Detecting the language once, then staying with it.
//
// Whisper decides the language of each clip independently, and a spoken turn is
// three or four seconds — far too little to be sure. Across a conversation the
// answer changes turn by turn, which produced "ней olurs sustainable change":
// Bulgarian, Turkish and English in one sentence, each fragment confidently
// labelled something different.
//
// Somebody switching language mid-conversation is rare. Whisper guessing wrong
// on a short clip is not. So the first confident detection is kept and reused,
// which turns many weak guesses into one better-informed one.

var (
	sessionLanguage string
	detectedMu      sync.RWMutex
)

// DetectedLanguage is what was decided for this session, if anything.
func DetectedLanguage() string {
	detectedMu.RLock()
	defer detectedMu.RUnlock()

	return sessionLanguage
}

// ForgetDetectedLanguage clears it, so the next turn decides afresh.
//
// Called when the owner changes the language setting: having said which
// language they are speaking, they should not be overruled by something
// detected before they said it.
func ForgetDetectedLanguage() {
	detectedMu.Lock()
	sessionLanguage = ""
	detectedMu.Unlock()
}

// languageForTurn is what to pass whisper for this clip.
func languageForTurn() string {
	if code := Language(); code != "" {
		return code
	}

	if code := DetectedLanguage(); code != "" {
		return code
	}

	return "auto"
}

// whisper reports the language it settled on, in a line that looks like:
// "auto-detected language: bg (p = 0.98)"
var detectedLine = regexp.MustCompile(`auto-detected language:\s*([a-z]{2,3})\s*\(p\s*=\s*([0-9.]+)\)`)

// DetectionConfidence below which a guess is not worth keeping.
//
// A weak guess repeated for a whole conversation is worse than a weak guess
// used once: the mistake stops being a glitch and becomes the setting.
const DetectionConfidence = 0.6

// rememberDetection keeps a confident detection for the rest of the session.
func rememberDetection(stderr string) {
	if Language() != "" || DetectedLanguage() != "" {
		return
	}

	match := detectedLine.FindStringSubmatch(stderr)
	if match == nil {
		return
	}

	if parseProbability(match[2]) < DetectionConfidence {
		return
	}

	detectedMu.Lock()
	sessionLanguage = match[1]
	detectedMu.Unlock()
}

func parseProbability(s string) float64 {
	var value float64
	var seenDot bool
	var scale = 0.1

	for _, r := range s {
		switch {
		case r == '.':
			seenDot = true
		case r >= '0' && r <= '9':
			digit := float64(r - '0')

			if seenDot {
				value += digit * scale
				scale /= 10

				continue
			}

			value = value*10 + digit
		}
	}

	return value
}

// detectLanguage asks whisper what language a clip is, without transcribing it.
//
// Used only when nothing is known yet. It costs a pass over the audio, which is
// worth paying once to avoid a conversation that changes language every turn.
func detectLanguage(ctx context.Context, wav string) string {
	r, _ := FindRecogniser()
	if r == nil {
		return ""
	}

	out, err := exec.CommandContext(ctx, r.Command,
		"-m", r.Model, "-f", wav, "-dl", "-np").CombinedOutput()
	if err != nil {
		return ""
	}

	match := detectedLine.FindStringSubmatch(string(out))
	if match == nil {
		return ""
	}

	if parseProbability(match[2]) < DetectionConfidence {
		return ""
	}

	return strings.TrimSpace(match[1])
}
