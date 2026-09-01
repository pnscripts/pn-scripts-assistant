package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"pn-brain/internal/brain/learning"
	"pn-brain/internal/brain/progress"
	"pn-brain/internal/brain/tools"
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
	"learn this",
	"learn it",
	"read and learn",
	"read this",
	"read it",
	"remember this",
	"remember it",
	"remember that",
	"look at this",
	"go through",
	"study",
	"index",
	"scan",

	// Said out loud, in Bulgarian, by somebody who thinks in it.
	"научи",
	"запомни",
	"прочети",
}

var (
	linkPattern = regexp.MustCompile(`https?://\S+`)

	/*
	 * A link said out loud, which never has a protocol in front of it.
	 *
	 * Nobody says "aitch tee tee pee colon slash slash" — they say
	 * "pnscripts.com" — and the recogniser writes down what they said. So an
	 * instruction given by voice never matched the pattern above, fell through
	 * to the model, and got the answer this whole file exists to prevent: "I
	 * will read that and learn from it", followed by nothing at all.
	 *
	 * A known suffix is required. Without one, any sentence containing a full
	 * stop between two words — which is most sentences the recogniser produces
	 * — would be read as a web address.
	 */
	bareLinkPattern = regexp.MustCompile(
		`\b((?:[a-zA-Z0-9][a-zA-Z0-9-]*\.)+` +
			`(?:com|net|org|io|dev|bg|co|uk|app|sh|me|ai|info|eu))` +
			`(/[^\s"']*)?\b`)

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

	/*
	 * And the same address without the protocol, which is how it arrives when
	 * it was spoken rather than pasted.
	 *
	 * https rather than http, because a site that only answers on http will
	 * redirect and one that only answers on https will not.
	 */
	if bare := bareLinkPattern.FindString(message); bare != "" {
		return LearnTarget{
			Link:  "https://" + strings.Trim(bare, `.,;)"'`),
			Found: true,
		}
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

		return b.learnFromPage(ctx, target.Link)
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

	/*
	 * Said before it starts, when it is going to take a while.
	 *
	 * Every observation is embedded on this processor — measured at roughly
	 * two seconds each — so a working folder full of documentation is an hour
	 * of work. Somebody who asked for it should have it, and should be told
	 * what they asked for rather than discovering it by watching a number that
	 * does not move. Announced rather than refused: they named the folder and
	 * said every document in it.
	 */
	if len(observations) > learning.ManyToLearn {
		progress.Set("learning", fmt.Sprintf(
			"Learning from %s — %d things, about %d minutes",
			filepath.Base(target.Path), len(observations),
			(len(observations)*learning.SecondsEachHere)/60))
	}

	/*
	 * How far along, not how much there is.
	 *
	 * Seen is set to the total before the first observation is touched, so
	 * "%d of %d" against it read "2386 of 2386" from the opening moment and
	 * never moved again — a progress line that shows completion at the start
	 * and then sits there for an hour is indistinguishable from a hang, which
	 * is the one thing it exists to rule out.
	 *
	 * Recorded and Rejected are the two ways an observation is finished with,
	 * so their sum is what has actually been dealt with.
	 */
	report, err := b.Learner.Ingest(ctx, observations, func(done learning.IngestReport) {
		progress.Set("learning", fmt.Sprintf("Learning from %s — %d of %d",
			filepath.Base(target.Path),
			done.Recorded+done.Rejected, len(observations)))
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

// learnFromPage reads a page and proposes what it found.
//
// Everything from a page waits for a person, and that is the design rather than
// caution for its own sake. The brain has read something written by somebody
// else; the difference between "this page says so" and "this is true about my
// owner" is exactly the judgement the review queue exists to hold. A brain that
// swallowed a page whole would fill its memory with other people's claims and
// then recall them as though they were its owner's.
func (b *Brain) learnFromPage(ctx context.Context, link string) (string, bool) {
	if b.Learner == nil {
		return "I have no learning pipeline running, so there is nowhere to put what I read.", true
	}

	progress.Set("learning", "Reading "+link)
	defer progress.Done()

	// The fetch tool rather than a plain request: it carries the guard against
	// being pointed at this machine's own network, and the size limit.
	args, err := json.Marshal(map[string]string{"url": link})
	if err != nil {
		return "I could not put that link together: " + err.Error(), true
	}

	page, err := tools.FetchURL{}.Execute(ctx, args)
	if err != nil {
		return fmt.Sprintf("I could not read %s: %v", link, err), true
	}

	text := strings.TrimSpace(tools.ReadableText(page))

	if len(text) < 200 {
		return fmt.Sprintf(
			"I fetched %s but there was almost no readable text on it — "+
				"probably a page that builds itself in the browser. "+
				"If you can point me at a plain article or a file, I can read that.", link), true
	}

	report, err := b.Learner.LearnFromText(ctx, text, link, func(at, total int) {
		progress.Set("learning", fmt.Sprintf("Reading %s — passage %d of %d", link, at, total))
	})
	if err != nil {
		return fmt.Sprintf("I read %s but could not finish learning from it: %v", link, err), true
	}

	if report.Recorded == 0 {
		return fmt.Sprintf(
			"I read %s — %d passages — and found nothing worth keeping. "+
				"Is there something in particular on it you wanted me to take away?",
			link, report.Seen), true
	}

	// Asked rather than announced. What is on the page is somebody else's
	// writing, and which parts of it are worth the brain believing is a
	// question with only one person who can answer it.
	return fmt.Sprintf(
		"I read %s and put %d things in your review queue — they are on the command centre "+
			"under \"Waiting for you\". Would you like to go through them now, or shall I "+
			"read more of the site first?",
		link, report.Recorded), true
}
