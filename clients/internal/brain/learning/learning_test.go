package learning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are the lessons that actually accumulated before the guard existed:
// seventeen pending, most of them the brain describing itself after a rename.
func TestGuardRejectsSelfDescription(t *testing.T) {
	reject := []string{
		"I am PN Scripts Assistant, a personal AI assistant.",
		"I can read files and run commands.",
		"I will remember things across conversations.",
		"My name is Vesper.",
		"The assistant helps Alex with his projects.",
		"As an AI, I do not have personal preferences.",

		// Every former name. Renaming left one stale identity claim each time.
		"Sage is a self-learning assistant built on Laravel.",
		"Vesper is Alex's personal AI assistant.",
		"Pnexus is a private brain that learns over time.",
		"PN Scripts Assistant is a personal, self-learning AI assistant.",

		// Wrapped claims: strip the wrapper before judging, or these sail past.
		"Remember that I am PN Scripts Assistant.",
		"remember: I can act on the computer.",
		"Note that I will ask before changing anything.",
		"The user should know that I am an AI assistant.",

		// A name the list has never seen.
		"Athena is a personal assistant that runs locally.",

		// Instructions echoed back as discoveries.
		"Prefer one purposeful call over several speculative ones.",
		"Say what you intend to do and why.",
	}

	for _, lesson := range reject {
		if !IsAboutTheAssistant(lesson, "PN Scripts Assistant", "Alex") {
			t.Errorf("guard let a self-description through: %q", lesson)
		}
	}
}

// The guard must not become so broad that real facts about the owner are lost.
// A rejected observation is silently gone; there is no queue for it.
func TestGuardKeepsFactsAboutTheOwner(t *testing.T) {
	keep := []string{
		"Alex has a Go project called xplorer-golang-api on his external drive.",
		"Alex prefers Laravel for web projects.",
		"Alex runs PN Scripts, a hosting and domains business in Sofia.",
		"Alex keeps his documents in /home/alex/Documents.",
		"Alex corrected the assistant: the organisation name is lowercase.",
		"Alex uses Ollama for local inference because the machine has no GPU.",

		// Mentions an assistant but is a fact about the owner's preference.
		"Alex prefers that the assistant asks for approval before running commands.",
	}

	for _, lesson := range keep {
		if IsAboutTheAssistant(lesson, "PN Scripts Assistant", "Alex") {
			t.Errorf("guard discarded a real fact about the owner: %q", lesson)
		}
	}
}

func TestStripCodeFence(t *testing.T) {
	cases := map[string]string{
		"{\"lesson\": null}":               "{\"lesson\": null}",
		"```json\n{\"lesson\": null}\n```": "{\"lesson\": null}",
		"```\n{\"lesson\": \"x\"}\n```":    "{\"lesson\": \"x\"}",
		"  ```json\n{\"a\": 1}\n```  ":     "{\"a\": 1}",
	}

	for in, want := range cases {
		if got := stripCodeFence(in); got != want {
			t.Errorf("stripCodeFence(%q) = %q, want %q", in, got, want)
		}
	}
}

// A filesystem claim is checked against the filesystem. A model's inference
// cannot be, so it waits for a person.
func TestValidatorChecksObservationsAgainstDisk(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "a-project")

	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}

	var v Validator

	cases := []struct {
		name   string
		source string
		want   string
	}{
		{"existing project", "project:" + real, StatusValidated},
		{"existing document", "document:" + real, StatusValidated},
		{"deleted since scanning", "project:" + filepath.Join(dir, "gone"), StatusRejected},
		{"model inference", "", StatusProposed},
		{"chat lesson", "conversation:12", StatusProposed},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := v.StatusFor(c.source); got != c.want {
				t.Errorf("StatusFor(%q) = %q, want %q", c.source, got, c.want)
			}
		})
	}
}

func TestOnlyScannedClaimsAreMachineVerifiable(t *testing.T) {
	var v Validator

	if !v.IsMachineVerifiable("project:/tmp") || !v.IsMachineVerifiable("document:/tmp") {
		t.Error("a scanned observation was reported as unverifiable")
	}

	// An inference about what the owner meant cannot be checked by anything
	// here; claiming otherwise would auto-promote a model's guess.
	if v.IsMachineVerifiable("") || v.IsMachineVerifiable("conversation:1") {
		t.Error("a model inference was reported as machine-verifiable")
	}
}

func TestCategoryFollowsTheSource(t *testing.T) {
	cases := map[string]string{
		"project:/a":      "project",
		"document:/b":     "document",
		"conversation:14": "conversation",
		"":                "conversation",
	}

	for source, want := range cases {
		if got := categoryFor(source); got != want {
			t.Errorf("categoryFor(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestExtractorPromptRefusesSelfDescriptionAndAllowsNothing(t *testing.T) {
	p := Extractor{Owner: "Alex", Name: "PN Scripts Assistant"}.Prompt("user: hello")

	// Finding nothing must be presented as normal, or a model asked to extract
	// a fact will always invent one.
	for _, required := range []string{
		"Record nothing about the assistant",
		"which is the common case",
		`{"lesson": null}`,
		"Alex",
	} {
		if !strings.Contains(p, required) {
			t.Errorf("prompt is missing %q", required)
		}
	}
}

// The twelve lessons actually sitting unreviewed in the real brain, verbatim.
//
// Six of them are the assistant describing itself, and the narrow version of
// the guard caught one. They are pinned here because they are what the guard is
// for, and because a rule written from invented examples drifts away from the
// output the model really produces.
func TestGuardAgainstTheRealPendingLessons(t *testing.T) {
	selfDescription := []string{
		"The tool will list directory contents by default when asked; use specific tool calls to access other information",
		"I should not proceed with actions without explaining my intent and gaining approval",
		"When answering a question, use the tool's call to provide more information than just the result",
		"Always be mindful of my capabilities and limitations, using tools when necessary",
		"I can recognize a simple command from Alex",
		"I should prioritize one-purposeful calls over speculative ones.",
	}

	for _, lesson := range selfDescription {
		if !IsAboutTheAssistant(lesson, "PN Scripts Assistant", "Alex") {
			t.Errorf("self-description survived: %q", lesson)
		}
	}

	aboutTheOwner := []string{
		"The user prefers Laravel over Python for backend projects",
		"When working on backend projects, the user prefers Laravel over Python.",
		"User prefers sarcastic responses",
		"Alex has two projects using Go, namely docs-saas-golang-api and xplorer-golang-api",
		"Alex has two projects with golang-api, xplorer-hub and xplorer-golang-api",
		"The folders within /media/alex/DEV/Projects/xplorer are separate projects",
	}

	for _, lesson := range aboutTheOwner {
		if IsAboutTheAssistant(lesson, "PN Scripts Assistant", "Alex") {
			t.Errorf("real fact about the owner was discarded: %q", lesson)
		}
	}
}

// Broadening the first-person rule to any sentence starting "I" risks throwing
// away real knowledge. Checked against the shape the scanners actually produce,
// which is every one of the 79 facts in the real store.
func TestGuardDoesNotDiscardScannedFacts(t *testing.T) {
	scanned := []string{
		`Alex has a Go project called "xplorer-golang-api" at /media/alex/DEV/Projects/xplorer/xplorer-golang-api, last modified 2026-03-07.`,
		`Alex has a PHP/Composer project called "pnscripts.com" at /home/alex/Projects/Laravel/pnscripts/pnscripts.com, last modified 2025-08-01.`,
		`Alex has a Word document called "XplorerServer.docx" at /home/alex/Documents/XplorerServer.docx, last modified 2024-02-11.`,
		`Alex has a Node.js project called "_dev" at /media/alex/DEV/Projects/divacon.bg/themes/aeonmarket/_dev, last modified 2025-08-19.`,
		// A README excerpt can contain anything, including first-person prose
		// written by whoever authored the project.
		`Alex has a Go project called "notes" at /home/alex/notes. README excerpt: # Notes. I wrote this to keep track of things.`,
	}

	for _, fact := range scanned {
		if IsAboutTheAssistant(fact, "PN Scripts Assistant", "Alex") {
			t.Errorf("scanned fact was discarded: %q", fact)
		}
	}
}

// Captured in practice: after answering "where does xplorer-golang-api live",
// the extractor proposed remembering the path it had just said. That is the
// model restating the conversation, not learning something about anybody.
func TestFragmentsAreNotFacts(t *testing.T) {
	notFacts := []string{
		"/media/alex/c8fc2986-4b79-4d7b-9a8c-e6db653915ac/DEV/Projects/xplorer/xplorer-golang-api",
		"xplorer-golang-api",
		"https://github.com/pnscripts/pn-scripts-assistant",
		"the path /home/alex/Projects",
		"",
		"   ",
		"Laravel",
		"yes",
	}

	for _, s := range notFacts {
		if !IsNotAFact(s) {
			t.Errorf("accepted a fragment as a fact: %q", s)
		}
	}
}

// The rule must not swallow real facts, which are often short.
func TestRealFactsSurviveTheFragmentCheck(t *testing.T) {
	facts := []string{
		"Alex prefers Laravel over Python",
		"Alex runs PN Scripts in Sofia",
		"The user prefers sarcastic responses",
		`Alex has a Go project called "xplorer-golang-api" at /media/alex/DEV/Projects/xplorer/xplorer-golang-api, last modified 2026-03-07.`,
		"Alex regularly uses the website pnscripts.local (1160 visits in browser history).",
	}

	for _, s := range facts {
		if IsNotAFact(s) {
			t.Errorf("discarded a real fact: %q", s)
		}
	}
}
