package brain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"pn-brain/internal/brain/learning"
	"pn-brain/internal/brain/progress"
)

// Being told to learn something is an instruction, not a topic of conversation.
//
// Asked to learn from a folder, the brain used to answer "I will familiarise
// myself with those resources and incorporate them into my learning process"
// and then do nothing whatsoever. That is the worst answer available: it is
// indistinguishable from having worked, so nobody checks, and the thing never
// gets learned. A model cannot be relied on to carry out an instruction it is
// capable of describing instead.
//
// So this is handled before the model sees it. The work happens, and what comes
// back is a count of what was actually recorded.

// learnPhrases are the ways of asking, kept narrow.
//
// All of them require a path or a link in the same sentence, so "I want to
// learn Go" — a statement about the owner, and a perfectly good thing to
// remember — is not mistaken for a command to go and read something.
var learnPhrases = []string{
	"learn from",
	"learn about",
	"learn everything",
	"read and learn",
	"study",
	"index",
	"scan",
}

var (
	linkPattern = regexp.MustCompile(`https?://\S+`)
	pathPattern = regexp.MustCompile(`(?:^|\s)(/[^\s"']+)`)
)

// LearnTarget is what the brain has been asked to learn from.
type LearnTarget struct {
	Path  string
	Link  string
	Found bool
}

// readLearnInstruction decides whether a message is an instruction to learn
// from something specific.
func readLearnInstruction(message string) LearnTarget {
	lower := strings.ToLower(message)

	asked := false

	for _, phrase := range learnPhrases {
		if strings.Contains(lower, phrase) {
			asked = true

			break
		}
	}

	if !asked {
		return LearnTarget{}
	}

	if link := linkPattern.FindString(message); link != "" {
		return LearnTarget{Link: strings.Trim(link, `.,;)"'`), Found: true}
	}

	// The path has to exist. A sentence that merely mentions a directory is not
	// an instruction to read it, and a typo should say so rather than sending
	// the brain off to scan the closest thing it can find.
	for _, match := range pathPattern.FindAllStringSubmatch(message, -1) {
		candidate := strings.Trim(match[1], `.,;)"'`)

		if _, err := os.Stat(candidate); err == nil {
			return LearnTarget{Path: candidate, Found: true}
		}
	}

	return LearnTarget{}
}

// handleLearnInstruction carries out an instruction to learn from something.
//
// Returns the answer and whether it handled the message.
func (b *Brain) handleLearnInstruction(ctx context.Context, message string) (string, bool) {
	target := readLearnInstruction(message)

	if !target.Found {
		return "", false
	}

	if target.Link != "" {
		// Saying plainly that this is switched off is better than fetching it
		// anyway, and much better than the model promising to read it.
		if !b.Mode.AllowsWeb() {
			return fmt.Sprintf(
				"I can't read %s: web access is off in %s mode. "+
					"Change the privacy mode and ask again, or point me at a file on this machine.",
				target.Link, b.Mode), true
		}

		return fmt.Sprintf(
			"I can read %s when you ask me a question about it, but I don't have a way to "+
				"take a whole site into memory yet. Point me at a folder or a file and I will "+
				"read that now.", target.Link), true
	}

	if b.Learner == nil {
		return "I have no learning pipeline running, so there is nowhere to put what I read.", true
	}

	progress.Set("learning", "Reading "+target.Path)
	defer progress.Done()

	observations, scanned, err := b.observe(target.Path)
	if err != nil {
		return fmt.Sprintf("I could not read %s: %v", target.Path, err), true
	}

	if len(observations) == 0 {
		return fmt.Sprintf("I read %s and found nothing worth recording.", target.Path), true
	}

	report, err := b.Learner.Ingest(ctx, observations, func(done learning.IngestReport) {
		progress.Set("learning", fmt.Sprintf("Learning from %s — %d of %d",
			filepath.Base(target.Path), done.Seen, len(observations)))
	})
	if err != nil {
		return fmt.Sprintf("I read %s but could not finish learning from it: %v", target.Path, err), true
	}

	return describeLearning(target.Path, scanned, report), true
}

// observe turns a path into things worth recording.
func (b *Brain) observe(path string) (learning.Observations, int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}

	owner := b.Cfg.Owner

	if !info.IsDir() {
		doc := learning.Document{
			Path: path,
			Name: info.Name(),
			Kind: strings.TrimPrefix(filepath.Ext(path), "."),
		}

		return learning.FromDocuments([]learning.Document{doc}, owner), 1, nil
	}

	projects, err := learning.ScanProjects(path)
	if err != nil {
		return nil, 0, err
	}

	documents, err := learning.ScanDocuments(path)
	if err != nil {
		return nil, 0, err
	}

	observations := append(
		learning.FromProjects(projects, owner),
		learning.FromDocuments(documents, owner)...)

	return observations, len(projects) + len(documents), nil
}

// describeLearning says what actually happened, in numbers.
//
// Counts rather than adjectives. "I have learned about your projects" is the
// sentence that made this necessary in the first place.
func describeLearning(path string, scanned int, report learning.IngestReport) string {
	var parts []string

	if report.Promoted > 0 {
		parts = append(parts, fmt.Sprintf("%d went straight into memory", report.Promoted))
	}

	if report.Waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d are waiting for you to confirm", report.Waiting))
	}

	if report.Duplicates > 0 {
		parts = append(parts, fmt.Sprintf("%d I already knew", report.Duplicates))
	}

	if report.Rejected > 0 {
		parts = append(parts, fmt.Sprintf("%d were rejected", report.Rejected))
	}

	if len(parts) == 0 {
		return fmt.Sprintf("I read %s — %d things — and none of them were worth keeping.", path, scanned)
	}

	return fmt.Sprintf("I read %s and found %d things. Of those, %s.",
		path, scanned, strings.Join(parts, ", "))
}
