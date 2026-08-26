package learning

import (
	"regexp"
	"strings"
)

// wrappers a model puts around a claim. Strip these before judging the claim,
// or "Remember that I am X" sails past a check for sentences beginning "I am".
var wrappers = []string{
	"remember that ",
	"remember: ",
	"note that ",
	"the user should know that ",
}

// firstPerson openers.
//
// The rule is broader than it looks and deliberately so: the extractor is asked
// for "one sentence about <owner>", and a sentence about somebody else does not
// begin with "I". Every lesson starting in first person is therefore the
// assistant talking about itself.
//
// The narrow version of this list — "i am", "i can", "i will" — was measured
// against twelve real pending lessons and caught one of six self-descriptions.
// It missed "I should not proceed with actions…" and "I should prioritize
// one-purposeful calls…" purely because "should" was not enumerated.
var firstPerson = []string{
	"i ", "i'", "my name is", "my capabilities", "my instructions",
	"the assistant ", "as an ai",
}

// Every name this project has carried. Renaming left one stale identity claim
// behind each time — "Sage is…", "Vesper is…" — and those are still
// self-description, just outdated.
var formerNames = []string{"sage", "vesper", "pnexus", "pn brain"}

// assistantSubject catches a sentence whose subject is an assistant under a
// name this list has never seen.
var assistantSubject = regexp.MustCompile(`^[a-z][a-z0-9 .-]{0,20} (is|can|uses|will|prefers) `)

// instructions that read as advice to the assistant rather than a fact about
// the owner.
//
// These are checked against the whole text and only count when the owner is not
// named, because "Petar prefers that the assistant asks first" is a real
// preference while "Always be mindful of my capabilities" is the assistant
// reciting its own prompt.
var instructions = []string{
	"prefer one purposeful call",
	"one-purposeful call",
	"say what you intend",
	"approval is necessary",
	"requires permission",
	"the tool will",
	"use the tool",
	"the tool's",
	"always be mindful",
	"my capabilities",
	"use tools when",
}

// IsAboutTheAssistant reports whether a proposed lesson describes the assistant
// rather than its owner.
//
// Written from what actually accumulated: seventeen pending lessons, of which
// most were the brain restating its own name after each rename ("I am Sage",
// "I am Vesper") or reciting its own instructions back as discoveries about the
// user.
//
// The extraction prompt already asks the model not to do this. This exists
// because a prompt is a request, and a small model asked to find something
// interesting will find something — usually the most prominent text in its
// context, which is its own instructions.
func IsAboutTheAssistant(lesson, assistantName, owner string) bool {
	text := strings.ToLower(strings.TrimSpace(lesson))

	for _, w := range wrappers {
		if strings.HasPrefix(text, w) {
			text = text[len(w):]

			break
		}
	}

	for _, opener := range firstPerson {
		if strings.HasPrefix(text, opener) {
			return true
		}
	}

	known := append([]string{}, formerNames...)

	if n := strings.ToLower(strings.TrimSpace(assistantName)); n != "" {
		known = append(known, n)
	}

	for _, k := range known {
		if strings.HasPrefix(text, k+" ") {
			return true
		}
	}

	if assistantSubject.MatchString(text) &&
		(strings.Contains(text, "ai assistant") || strings.Contains(text, "personal assistant")) {
		return true
	}

	isInstruction := false

	for _, phrase := range instructions {
		if strings.Contains(text, phrase) {
			isInstruction = true

			break
		}
	}

	// An instruction that names the owner may genuinely be about them.
	return isInstruction && !strings.Contains(text, strings.ToLower(owner))
}

// stripCodeFence removes the ```json wrapper models add despite being asked for
// bare JSON.
func stripCodeFence(text string) string {
	text = strings.TrimSpace(text)

	if !strings.HasPrefix(text, "```") {
		return text
	}

	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[i+1:]
	}

	text = strings.TrimSuffix(strings.TrimSpace(text), "```")

	return strings.TrimSpace(text)
}
