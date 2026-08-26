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
)

// Engine is a text-to-speech program available on this machine.
type Engine struct {
	Name    string
	Command string
	Args    func(text string) []string
}

// engines in preference order: quality first, then whatever is present.
var engines = []Engine{
	{
		// Neural voices, and by far the best sounding of these. Not installed
		// by default anywhere, so it is looked for rather than assumed.
		Name:    "piper",
		Command: "piper",
		Args:    func(text string) []string { return []string{"--output-raw"} },
	},
	{
		// speech-dispatcher: present on most desktop Linux installs, and it
		// routes through whatever the desktop already has configured.
		Name:    "speech-dispatcher",
		Command: "spd-say",
		Args:    func(text string) []string { return []string{"--", text} },
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
func Available() *Engine {
	once.Do(func() {
		for i := range engines {
			if _, err := exec.LookPath(engines[i].Command); err == nil {
				detected = &engines[i]

				return
			}
		}
	})

	return detected
}

// MaxSpokenChars bounds one utterance.
//
// A long answer read aloud in a synthetic voice is not useful — nobody listens
// to four minutes of it, and there is no way to skim. The reply is on screen;
// this is for hearing the gist without looking.
const MaxSpokenChars = 600

// Speak reads text aloud, returning once the engine has been handed the text.
//
// It does not wait for the speaking to finish. A reply that takes thirty
// seconds to read must not hold an HTTP request open for thirty seconds, and
// there is nothing useful to report at the end anyway.
func Speak(ctx context.Context, text string) error {
	engine := Available()
	if engine == nil {
		return fmt.Errorf(
			"no speech engine found. On Linux install one with: " +
				"sudo apt install speech-dispatcher espeak-ng")
	}

	spoken := Readable(text)
	if spoken == "" {
		return nil
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
