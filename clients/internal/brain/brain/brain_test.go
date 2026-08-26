package brain

import (
	"strings"
	"testing"

	"pn-brain/internal/brain/store"
)

// Reply time on a CPU is close to linear in prompt size, and recall is the
// biggest part of that prompt.
func TestRecallIsTrimmedForThePrompt(t *testing.T) {
	long := `Petar has a PHP/Composer project called "ps_emailsubscription" at ` +
		`/media/petar/DEV/Projects/divacon.bg/modules/ps_emailsubscription, last modified 2025-08-19. ` +
		`README excerpt: # E-mail subscription form [![Build Status](https://travis-ci.com/x.svg)](https://t) ` +
		strings.Repeat("more badge noise ", 40)

	out := formatRecall([]store.Scored{{Fact: store.Fact{Content: long}}})

	if len([]rune(out)) > RecalledFactLimit+200 {
		t.Errorf("recall preamble is %d runes; trimming did not happen", len([]rune(out)))
	}

	// The part that answers questions must survive.
	for _, want := range []string{"ps_emailsubscription", "/media/petar/DEV/Projects", "2025-08-19"} {
		if !strings.Contains(out, want) {
			t.Errorf("trimming removed %q, which is the useful part", want)
		}
	}

	// The part that is noise must not.
	if strings.Contains(out, "travis-ci") {
		t.Error("a build-status badge reached the prompt")
	}
}

// A fact cut mid-word reads as though it were complete and slightly wrong.
func TestTrimPrefersASentenceBoundary(t *testing.T) {
	s := "Petar has a Go project called xplorer at /home/petar/x, last modified 2026-01-01. " +
		strings.Repeat("trailing detail that should go away ", 20)

	got := trimForPrompt(s, 120)

	if !strings.HasSuffix(got, ".") {
		t.Errorf("did not end at a sentence: %q", got)
	}

	if strings.Contains(got, "trailing detail") {
		t.Errorf("kept the tail: %q", got)
	}
}

// Short facts must pass through untouched, or every website fact would gain a
// pointless ellipsis.
func TestShortFactsAreNotTrimmed(t *testing.T) {
	s := "Petar regularly uses the website pnscripts.local (1160 visits in browser history)."

	if got := trimForPrompt(s, RecalledFactLimit); got != s {
		t.Errorf("a short fact was altered:\n  got  %q\n  want %q", got, s)
	}
}
