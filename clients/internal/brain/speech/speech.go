// Package speech lets the brain read its answers aloud.
//
// Local engines only, and that is a privacy decision rather than a technical
// one. A cloud voice would mean every reply the brain composes — including
// anything it recalled from this machine — being posted to a third party to be
// turned into audio, which is exactly what the privacy rules exist to prevent.
// A local voice is worse-sounding and does not do that.
//
// Speaking is the half of "voice" that can be done with what a desktop already
// has. Listening needs a speech model that is not installed by default and is
// several gigabytes; see Listening.
package speech

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Engine is a text-to-speech program available on this machine.
type Engine struct {
	Name    string
	Command string
	Args    func(text string) []string

	// NeedsEngine marks a client that only forwards text to something else,
	// and is therefore no evidence that anything can speak.
	NeedsEngine bool
}

// engines in preference order: quality first, then whatever is present.
var engines = []Engine{
	{
		// speech-dispatcher: present on most desktop Linux installs, and it
		// routes through whatever the desktop already has configured.
		Name:        "speech-dispatcher",
		Command:     "spd-say",
		Args:        func(text string) []string { return []string{"--", text} },
		NeedsEngine: true,
	},
	{
		Name:    "espeak-ng",
		Command: "espeak-ng",
		Args:    func(text string) []string { return []string{"--", text} },
	},
	{
		Name:    "espeak",
		Command: "espeak",
		Args:    func(text string) []string { return []string{"--", text} },
	},
	{
		// macOS ships this.
		Name:    "say",
		Command: "say",
		Args:    func(text string) []string { return []string{text} },
	},
}

var (
	once     sync.Once
	detected *Engine
)

// Available returns the engine this machine can use, or nil.
//
// spd-say being present is not enough, and assuming it was is why this brain
// reported speech as a capability while producing silence. spd-say is a client:
// it hands text to speech-dispatcher and exits 0 whether or not anything can
// say it. On this machine speech-dispatcher had its espeak-ng adapter but not
// the espeak-ng engine, so it fell back to sd_dummy — a module whose entire
// purpose is to accept speech and make no sound.
//
// So a real voice is required behind the client.
func Available() *Engine {
	once.Do(func() {
		// A neural voice if one is installed. espeak-ng is formant synthesis
		// and sounds it; piper is what somebody would actually want reading
		// answers to them.
		if p := FindPiper(); p != nil {
			detected = &Engine{Name: "piper", Command: p.Binary}

			return
		}

		for i := range engines {
			if _, err := exec.LookPath(engines[i].Command); err != nil {
				continue
			}

			if engines[i].NeedsEngine && !anyVoiceInstalled() {
				continue
			}

			detected = &engines[i]

			return
		}
	})

	return detected
}

// voices are the programs that turn text into sound. speech-dispatcher drives
// one of these; with none of them it drives sd_dummy and says nothing.
var voices = []string{"espeak-ng", "espeak", "pico2wave", "flite"}

func anyVoiceInstalled() bool {
	for _, v := range voices {
		if _, err := exec.LookPath(v); err == nil {
			return true
		}
	}

	return false
}

// MaxSpokenChars bounds one utterance.
//
// A long answer read aloud in a synthetic voice is not useful — nobody listens
// to four minutes of it, and there is no way to skim. The reply is on screen;
// this is for hearing the gist without looking.
const MaxSpokenChars = 600

// SpeakAndWait reads text aloud and returns when it has finished being said.
//
// Conversation mode needs this rather than Speak. On speakers — not headphones
// — the microphone hears whatever the brain is saying, so listening must not
// resume until the voice has actually stopped. Estimating that from the word
// count, which is what this replaced, is a guess that is wrong in both
// directions: too short and the brain transcribes itself, too long and every
// exchange drags.
//
// The waiting is done by the engine. spd-say -w returns when the message has
// been spoken; the others block for as long as they are speaking anyway.
func SpeakAndWait(ctx context.Context, text string) error {
	engine := Available()
	if engine == nil {
		return fmt.Errorf(
			"no speech engine found. speech-dispatcher on its own only queues text; " +
				"it needs a voice behind it. On Linux: sudo apt install espeak-ng")
	}

	spoken := Readable(text)
	if spoken == "" {
		return nil
	}

	// A chosen system voice overrides piper even when piper is installed.
	if engine.Name == "piper" && CurrentVoice().Engine == "piper" {
		return FindPiper().Speak(ctx, spoken)
	}

	if engine.Name == "piper" {
		if fallback := fallbackEngine(); fallback != nil {
			engine = fallback
		}
	}

	args := engine.Args(spoken)

	if engine.Command == "spd-say" {
		// -w waits for the message to be spoken rather than queueing it.
		args = append([]string{"-w"}, args...)
	}

	// A cap, so a stuck engine cannot hold a conversation open forever.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	return exec.CommandContext(ctx, engine.Command, args...).Run()
}

// Speak reads text aloud, returning once the engine has been handed the text.
//
// It does not wait for the speaking to finish. A reply that takes thirty
// seconds to read must not hold an HTTP request open for thirty seconds, and
// there is nothing useful to report at the end anyway.
func Speak(ctx context.Context, text string) error {
	engine := Available()
	if engine == nil {
		return fmt.Errorf(
			"no speech engine found. speech-dispatcher on its own only queues text; " +
				"it needs a voice behind it. On Linux: sudo apt install espeak-ng")
	}

	spoken := Readable(text)
	if spoken == "" {
		return nil
	}

	if engine.Name == "piper" && CurrentVoice().Engine == "piper" {
		// Piper synthesises faster than it plays, so there is nothing to gain
		// from returning early and a microphone to protect by not doing so.
		go FindPiper().Speak(context.WithoutCancel(ctx), spoken)

		return nil
	}

	if engine.Name == "piper" {
		if fallback := fallbackEngine(); fallback != nil {
			engine = fallback
		}
	}

	cmd := exec.CommandContext(ctx, engine.Command, engine.Args(spoken)...)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start %s: %w", engine.Name, err)
	}

	// Reaped in the background so the process does not become a zombie, and so
	// nothing here blocks on however long the sentence takes to say.
	go cmd.Wait()

	return nil
}

var (
	codeFence  = regexp.MustCompile("(?s)```.*?```")
	inlineCode = regexp.MustCompile("`([^`]*)`")
	urls       = regexp.MustCompile(`https?://\S+`)
	markdown   = regexp.MustCompile(`[*_#>|]+`)
	longPath   = regexp.MustCompile(`(/[^\s/]+){3,}/?`)
	spaces     = regexp.MustCompile(`\s+`)
)

// Readable strips what should not be read aloud.
//
// A path spoken character by character is unbearable and conveys nothing, and a
// code block read as prose is worse. Both are on screen, where they can be read
// properly; what is worth hearing is the sentence around them.
func Readable(text string) string {
	s := codeFence.ReplaceAllString(text, " (code) ")
	s = inlineCode.ReplaceAllString(s, "$1")
	s = urls.ReplaceAllString(s, " a link ")
	s = longPath.ReplaceAllString(s, " that path ")
	s = markdown.ReplaceAllString(s, " ")
	s = spaces.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	if r := []rune(s); len(r) > MaxSpokenChars {
		cut := string(r[:MaxSpokenChars])

		// End on a sentence rather than mid-word, which sounds like a fault.
		if i := strings.LastIndex(cut, ". "); i > MaxSpokenChars/2 {
			return cut[:i+1]
		}

		return cut
	}

	return s
}

// Listening reports whether the brain can hear, and why not when it cannot.
func Listening() (bool, string) {
	r, why := FindRecogniser()

	return r != nil, why
}
