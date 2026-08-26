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
		"I am PN Brain, a personal AI assistant.",
		"I can read files and run commands.",
		"I will remember things across conversations.",
		"My name is Vesper.",
		"The assistant helps Petar with his projects.",
		"As an AI, I do not have personal preferences.",

		// Every former name. Renaming left one stale identity claim each time.
		"Sage is a self-learning assistant built on Laravel.",
		"Vesper is Petar's personal AI assistant.",
		"Pnexus is a private brain that learns over time.",
		"PN Brain is a personal, self-learning AI assistant.",

		// Wrapped claims: strip the wrapper before judging, or these sail past.
		"Remember that I am PN Brain.",
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
		if !IsAboutTheAssistant(lesson, "PN Brain", "Petar") {
			t.Errorf("guard let a self-description through: %q", lesson)
		}
	}
}

// The guard must not become so broad that real facts about the owner are lost.
// A rejected observation is silently gone; there is no queue for it.
func TestGuardKeepsFactsAboutTheOwner(t *testing.T) {
	keep := []string{
		"Petar has a Go project called xplorer-golang-api on his external drive.",
		"Petar prefers Laravel for web projects.",
		"Petar runs PN Scripts, a hosting and domains business in Sofia.",
		"Petar keeps his documents in /home/petar/Documents.",
		"Petar corrected the assistant: the organisation name is lowercase.",
		"Petar uses Ollama for local inference because the machine has no GPU.",

		// Mentions an assistant but is a fact about the owner's preference.
		"Petar prefers that the assistant asks for approval before running commands.",
	}

	for _, lesson := range keep {
		if IsAboutTheAssistant(lesson, "PN Brain", "Petar") {
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
	p := Extractor{Owner: "Petar", Name: "PN Brain"}.Prompt("user: hello")

	// Finding nothing must be presented as normal, or a model asked to extract
	// a fact will always invent one.
	for _, required := range []string{
		"Record nothing about the assistant",
		"which is the common case",
		`{"lesson": null}`,
		"Petar",
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
		"I can recognize a simple command from Petar",
		"I should prioritize one-purposeful calls over speculative ones.",
	}

	for _, lesson := range selfDescription {
		if !IsAboutTheAssistant(lesson, "PN Brain", "Petar") {
			t.Errorf("self-description survived: %q", lesson)
		}
	}

	aboutTheOwner := []string{
		"The user prefers Laravel over Python for backend projects",
		"When working on backend projects, the user prefers Laravel over Python.",
		"User prefers sarcastic responses",
		"Petar has two projects using Go, namely docs-saas-golang-api and xplorer-golang-api",
		"Petar has two projects with golang-api, xplorer-hub and xplorer-golang-api",
		"The folders within /media/petar/DEV/Projects/xplorer are separate projects",
	}

	for _, lesson := range aboutTheOwner {
		if IsAboutTheAssistant(lesson, "PN Brain", "Petar") {
			t.Errorf("real fact about the owner was discarded: %q", lesson)
		}
	}
}

// Broadening the first-person rule to any sentence starting "I" risks throwing
// away real knowledge. Checked against the shape the scanners actually produce,
// which is every one of the 79 facts in the real store.
func TestGuardDoesNotDiscardScannedFacts(t *testing.T) {
	scanned := []string{
		`Petar has a Go project called "xplorer-golang-api" at /media/petar/DEV/Projects/xplorer/xplorer-golang-api, last modified 2026-03-07.`,
		`Petar has a PHP/Composer project called "pnscripts.com" at /home/petar/Projects/Laravel/pnscripts/pnscripts.com, last modified 2025-08-01.`,
		`Petar has a Word document called "XplorerServer.docx" at /home/petar/Documents/XplorerServer.docx, last modified 2024-02-11.`,
		`Petar has a Node.js project called "_dev" at /media/petar/DEV/Projects/divacon.bg/themes/aeonmarket/_dev, last modified 2025-08-19.`,
		// A README excerpt can contain anything, including first-person prose
		// written by whoever authored the project.
		`Petar has a Go project called "notes" at /home/petar/notes. README excerpt: # Notes. I wrote this to keep track of things.`,
	}

	for _, fact := range scanned {
		if IsAboutTheAssistant(fact, "PN Brain", "Petar") {
			t.Errorf("scanned fact was discarded: %q", fact)
		}
	}
}
