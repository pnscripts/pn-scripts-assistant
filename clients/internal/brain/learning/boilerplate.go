package learning

import (
	"strings"
	"sync"
)

/*
 * A line that appears in many documents is a template, not a fact.
 *
 * This is the rule that Petar's queue was asking for. Grouped by folder, the
 * 9,199 things waiting for him were dominated by files that repeat: a manual
 * whose every page carries the same banner, a mail template kept in thirty
 * languages, a vendored library whose schema files share a licence header, a
 * folder of invoices carrying the same VAT note at the foot of each one. Every
 * copy arrived as a separate thing to remember, and each was perfectly true.
 *
 * Judged on the line itself rather than on the sentence built around it. The
 * stored form is "<name>, a PDF document at <path>, says: <line>", which is
 * different for every file — so nothing downstream that compares whole
 * observations, by text or by meaning, can see that the interesting half is
 * identical. It has to be caught here, where the line is still a line.
 *
 * In memory and for the life of the process, which is the right span: a pass
 * over a drive is one process, and a document already read is not opened again
 * on the next one.
 */
type seenLines struct {
	mu sync.Mutex

	// How many distinct documents each line has turned up in.
	in map[string]int

	// Which document last counted a line, so a sentence repeated within one
	// file — a refrain, a table header on every page — counts once for it.
	from map[string]string
}

var repeated = &seenLines{in: map[string]int{}, from: map[string]string{}}

/*
 * boilerplate records that a line was found in a document and reports whether
 * it has been found in too many.
 *
 * The counting happens whether or not the line is kept, so the third document
 * to carry it is the last one that does.
 */
func (s *seenLines) boilerplate(line, document string) bool {
	key := fingerprint(line)

	if key == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.from[key] != document {
		s.from[key] = document
		s.in[key]++
	}

	return s.in[key] > SeenInThisManyDocuments
}

// Forget empties the record. For tests, and for a fresh pass that should judge
// repetition on its own evidence rather than on a previous run's.
func (s *seenLines) Forget() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.in = map[string]int{}
	s.from = map[string]string{}
}

/*
 * fingerprint is the line stripped of what varies between copies of it.
 *
 * Case and spacing, because a template rendered twice differs in neither in
 * any way that matters. Digits, because an invoice number, a year and a page
 * number are exactly the parts that change while the sentence around them
 * stays the same — "Page 3 of 12" and "Page 7 of 12" are one line.
 */
func fingerprint(line string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(line) {
		switch {
		case r >= '0' && r <= '9':
			continue
		default:
			b.WriteRune(r)
		}
	}

	return strings.Join(strings.Fields(b.String()), " ")
}
