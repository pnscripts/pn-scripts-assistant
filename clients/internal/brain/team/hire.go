package team

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Hiring: from "I need someone who specialises in Laravel security" to a
 * person on the organisation who does.
 *
 * In code, from the taxonomy, and never by asking a model to invent somebody.
 * The specification's own sequence, in order:
 *
 *   find the job         what the sentence names, matched against the jobs
 *                        and the capabilities the catalogue already has
 *   or find the person   somebody already here who can do all of it is the
 *                        answer, and hiring a copy of them is refused — two
 *                        agents that differ in name make the planner's
 *                        choice harder for nothing
 *   take a template      what that job may touch, when one exists
 *   add what is special  the capabilities the sentence named beyond the job
 *   decide the tools     never more than whoever asked could use
 *   find or make a seat  a vacant one for that job, or a new one beside the
 *                        people who do similar work
 *   save it              a file in the agents folder, like every agent
 *
 * A temporary hire, for one task, gets no seat: it is dissolved when the task
 * ends, and a chart that grew a seat for every one would be a chart nobody
 * could read by the end of the month.
 */

// Catalogue is what hiring needs from the taxonomy.
type Catalogue interface {
	Occupations
	FindOccupations(text string, limit int) ([]store.Occupation, error)
	FindCapabilities(text string, limit int) ([]store.Capability, error)
	OccupationsNeeding(capabilityID string, limit int) ([]store.Occupation, error)
}

// Wish is somebody being asked for.
type Wish struct {
	// Sentence is what was said, or a step's instruction, or a job title a
	// planner wrote.
	Sentence string

	// ForTask makes this a temporary hire for that task. Zero is a permanent
	// one, which only its owner asks for.
	ForTask int64

	// Within is the most the hire may use, when whoever asked was held to
	// less than everything. Nil is no limit.
	Within *[]string

	// Package is a capability package the hire is for, already chosen — a
	// project proposal's — so the role is the package's, named as it names it.
	Package string

	// Job is the job, named outright by its id — its owner picked it from
	// the catalogue — so nothing is matched from words.
	Job string

	// New asks for somebody new even when somebody here already does this:
	// its owner pressed "take them on", not "who does this".
	New bool
}

// Hired is what came of a wish.
type Hired struct {
	Agent Agent `json:"agent"`

	// Proposal is the hire that was worked out and not made, when it needed
	// its owner's approval.
	Proposal *Proposal `json:"proposal,omitempty"`

	// Existing is true when somebody already here was the answer.
	Existing bool `json:"existing"`

	Job  string `json:"job"`
	Seat string `json:"seat,omitempty"`

	// Can is what the sentence named that the job did not already cover.
	Can []string `json:"can,omitempty"`

	// Why is the decision in a sentence, for the report and the interface.
	Why string `json:"why"`
}

// asking is the words that say somebody is wanted rather than what for.
var asking = map[string]bool{
	"i": true, "we": true, "need": true, "want": true, "someone": true, "somebody": true,
	"who": true, "that": true, "can": true, "a": true, "an": true, "the": true, "to": true,
	"create": true, "hire": true, "make": true, "add": true, "get": true, "find": true,
	"agent": true, "person": true, "specialist": true, "specialists": true, "expert": true,
	"specialises": true, "specializes": true, "specialised": true, "specialized": true,
	"specialising": true, "specializing": true, "in": true, "on": true, "at": true,
	"of": true, "for": true, "with": true, "and": true, "is": true, "good": true,
	"knows": true, "know": true, "please": true, "me": true, "us": true, "new": true,
	"do": true, "does": true, "doing": true,
	"някой": true, "специалист": true, "агент": true, "експерт": true, "който": true,
	"трябва": true, "ми": true, "ни": true, "създай": true, "наеми": true, "за": true,
	"в": true, "на": true, "с": true, "и": true, "търся": true,
}

/*
 * stem is a word without the ending that says how it is being used.
 *
 * "Beekeeping" and "beekeeper" are one trade, and a catalogue that knows the
 * beekeeper answered "someone who knows beekeeping" with nobody. Crude on
 * purpose — a handful of English endings, and only when enough of the word is
 * left to mean something — because a wrong stem costs a weaker match, and a
 * clever one would be a dependency this program has always refused.
 */
func stem(word string) string {
	r := []rune(word)

	for _, ending := range []string{"ings", "ing", "ers", "er", "ists", "ist", "ments", "ment", "ions", "ion", "ies", "s"} {
		e := []rune(ending)

		if len(r)-len(e) >= 4 && strings.HasSuffix(word, ending) {
			return string(r[:len(r)-len(e)])
		}
	}

	return word
}

// subjectOf is the words of a wish that say what the work is.
func subjectOf(sentence string) []string {
	var words []string

	for _, w := range strings.FieldsFunc(strings.ToLower(sentence), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '#'
	}) {
		if !asking[w] && len([]rune(w)) > 1 {
			words = append(words, w)
		}
	}

	return words
}

/*
 * capabilitiesIn is the capabilities a wish names, pairs of words first.
 *
 * "Laravel security" is two capabilities and "machine learning" is one, and
 * the only way to tell is to try the pair before its halves. A word counts
 * when it is a capability's name, id or alias outright, or the start of its
 * name — "security" finds "Security auditing", "postgres" finds PostgreSQL.
 */
func capabilitiesIn(cat Catalogue, words []string) []string {
	found := []string{}
	used := make([]bool, len(words))

	try := func(term string) (string, bool) {
		list, err := cat.FindCapabilities(term, 3)
		if err != nil {
			return "", false
		}

		id := strings.ReplaceAll(term, " ", "_")

		for _, c := range list {
			name := strings.ToLower(c.Name)

			/*
			 * The whole name, or the start of a name that is one word.
			 *
			 * "postgres" is PostgreSQL. But with the classifications read in
			 * there are fourteen thousand skills, and the start of any name
			 * at all made "nurse" the skill of nursing plants and "translate"
			 * the translating of artistic concepts into technical designs.
			 */
			// The start of a name only when the word is long enough to mean
			// it: "postgres" is PostgreSQL, and "book" is not bookkeeping.
			if c.ID == id || name == term || stem(name) == term ||
				(!strings.Contains(name, " ") && len([]rune(term)) >= 5 && strings.HasPrefix(name, term)) {
				return c.ID, true
			}

			for _, alias := range c.Aliases {
				if strings.ToLower(alias) == term {
					return c.ID, true
				}
			}
		}

		return "", false
	}

	add := func(id string) {
		for _, have := range found {
			if have == id {
				return
			}
		}

		found = append(found, id)
	}

	for i := 0; i+1 < len(words); i++ {
		if id, ok := try(words[i] + " " + words[i+1]); ok {
			add(id)
			used[i], used[i+1] = true, true
		}
	}

	for i, w := range words {
		if used[i] || len([]rune(w)) < 3 {
			continue
		}

		if id, ok := try(w); ok {
			add(id)
		} else if s := stem(w); s != w {
			if id, ok := try(s); ok {
				add(id)
			}
		}
	}

	return found
}

/*
 * jobFor is the job a wish is best answered by.
 *
 * Scored word by word against what each job is. A job calling for a
 * capability the word names counts most — what a job needs is what it is —
 * then the word in its title, then in what else it is called.
 *
 * And each word counts for less the more jobs it matches. With the real
 * classifications read in, "security" is in a hundred jobs and "Laravel" in
 * one, and counting them equally hired a security alarm investigator for
 * Laravel security and a business analyst for PostgreSQL performance: the
 * common word matched something everywhere and the telling one was outvoted.
 * Weighted by rarity, the word that says what the work is decides it.
 *
 * A job that runs something rather than doing it counts for half, unless the
 * wish asked for one — asked for somebody in Laravel security, the chief
 * security officer matched as well as any engineer. Among equals, a job this
 * program ships or somebody wrote by hand goes before an imported one, since
 * those were written for exactly this.
 *
 * A job that matched nothing is not a weak answer; it is no answer, and
 * nobody is hired on the strength of it.
 */
func jobFor(cat Catalogue, words, capabilities []string) (*store.Occupation, error) {
	candidates := map[string]bool{}

	note := func(jobs []store.Occupation, err error) error {
		for _, job := range jobs {
			candidates[job.ID] = true
		}

		return err
	}

	for _, capability := range capabilities {
		if err := note(cat.OccupationsNeeding(capability, 60)); err != nil {
			return nil, err
		}
	}

	stems := make([]string, 0, len(words))

	for _, w := range words {
		w = stem(w)
		stems = append(stems, w)

		if err := note(cat.FindOccupations(w, 40)); err != nil {
			return nil, err
		}

		related, err := cat.FindCapabilities(w, 10)
		if err != nil {
			return nil, err
		}

		for _, c := range related {
			if strings.Contains(c.ID, w) || strings.Contains(strings.ToLower(c.Name), w) {
				if err := note(cat.OccupationsNeeding(c.ID, 40)); err != nil {
					return nil, err
				}
			}
		}
	}

	leading := leadingWish(words)

	jobs := make([]store.Occupation, 0, len(candidates))

	for id := range candidates {
		if job, err := cat.Occupation(id); err == nil && job != nil {
			jobs = append(jobs, *job)
		}
	}

	/*
	 * The word that names the kind of worker decides the kind of worker.
	 *
	 * Word by word, "a security guard" found the application security
	 * engineer — "security" is most of his job — and "a financial advisor"
	 * the academic one. The last word of "security guard" is what somebody
	 * is; the first is what about. Only jobs called that are considered, and
	 * when the catalogue has none, nobody is hired and the answer says so,
	 * rather than somebody else being hired under the name.
	 */
	if head := roleNoun(words); head != "" {
		kept := jobs[:0]

		for _, job := range jobs {
			if calledA(job, head) {
				kept = append(kept, job)
			}
		}

		if len(kept) == 0 {
			return nil, nil
		}

		jobs = kept
	}

	// How many candidates each word matches at all, for its weight.
	matching := make([]int, len(stems))

	for _, job := range jobs {
		for i, w := range stems {
			if pointsFor(job, w) > 0 {
				matching[i]++
			}
		}
	}

	var best *store.Occupation

	bestScore := 0.0

	for i := range jobs {
		job := jobs[i]
		score := 0.0

		for k, w := range stems {
			if points := pointsFor(job, w); points > 0 {
				score += float64(points) / math.Log2(2+float64(matching[k]))
			}
		}

		if score == 0 {
			continue
		}

		/*
		 * A wish that is exactly a job's name is that job.
		 *
		 * Word by word, "product manager" scored the chief product officer
		 * higher than the product manager — both words are in what a CPO
		 * needs — and a planner naming product_manager got a CPO. The whole
		 * phrase being the name settles it before any word is weighed.
		 */
		if named := strings.Join(words, " "); strings.EqualFold(job.Title, named) {
			score += 100
		} else {
			for _, alias := range job.Aliases {
				if strings.EqualFold(alias, named) {
					score += 50

					break
				}
			}

			score += phraseIn(words, job)
		}

		if !leading && leads(job) {
			score /= 2
		}

		if job.CameFrom == store.FromSeed || job.CameFrom == store.FromHand {
			score *= 1.1
		}

		if best == nil || score > bestScore || (score == bestScore && job.ID < best.ID) {
			best, bestScore = &jobs[i], score
		}
	}

	return best, nil
}

/*
 * phraseIn is a job's name of two words or more, said whole inside the wish.
 *
 * "A three.js game developer" is a game developer who works in three.js, and
 * word by word "three" found the 3D artist first. A title of several words
 * said in order is as good as the whole wish being the title, nearly: it
 * counts for a little less, so a wish that is exactly a job still wins.
 */
func phraseIn(words []string, job store.Occupation) float64 {
	said := " " + strings.Join(words, " ") + " "

	for i, name := range append([]string{job.Title}, job.Aliases...) {
		name = strings.ToLower(strings.TrimSpace(name))

		if !strings.Contains(name, " ") {
			continue
		}

		if strings.Contains(said, " "+name+" ") {
			if i == 0 {
				return 60
			}

			return 40
		}
	}

	return 0
}

/*
 * pointsFor is how much one word says a job is the one.
 *
 * Added up rather than the best of them: a job that needs security and is
 * called chief security officer says "security" twice, and one that only
 * needs it says it once. And a word that is the whole of a name counts for
 * more than one found inside it — "заварчик" is exactly one of the welder's
 * names and merely somewhere in the maintenance technician's.
 */
func pointsFor(job store.Occupation, w string) int {
	points := 0

	for _, need := range job.Needs {
		if wordIn(strings.ReplaceAll(need.ID, "_", " "), w) {
			points += 3

			break
		}
	}

	title := strings.ToLower(job.Title)

	switch {
	case title == w || stem(title) == w:
		points += 4
	case wordIn(title, w):
		points += 2
	}

	for _, alias := range job.Aliases {
		alias = strings.ToLower(alias)

		if alias == w || stem(alias) == w {
			points += 3

			break
		}

		if wordIn(alias, w) {
			points++

			break
		}
	}

	return points
}

/*
 * wordIn is whether a word is one of the words of a name — or the start of
 * one, when it is long enough to mean it: "postgres" is PostgreSQL.
 *
 * Words, not letters. Matched anywhere in the name, "book" was in bookkeeper
 * and "a book writer" hired somebody to keep the accounts; the part of a word
 * is a different word, and only a long enough start of one is the same.
 */
func wordIn(text, w string) bool {
	for _, word := range strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '+' && r != '#'
	}) {
		if word == w || stem(word) == w || stem(word) == stem(w) {
			return true
		}

		if len([]rune(w)) >= 5 && strings.HasPrefix(word, w) {
			return true
		}
	}

	return false
}

// leadingWish is whether the wish asks for somebody to run something.
func leadingWish(words []string) bool {
	for _, w := range words {
		switch w {
		case "chief", "head", "director", "manager", "lead", "officer", "executive", "vp":
			return true
		}
	}

	return false
}

// leads is whether a job is running something rather than doing it: filed as
// leadership, or with a title that says so wherever it is filed — the
// catalogue's security manager lives under safety.
func leads(job store.Occupation) bool {
	if strings.HasPrefix(job.ID, "leadership.") {
		return true
	}

	for _, w := range strings.Fields(strings.ToLower(job.Title)) {
		switch w {
		case "chief", "head", "director", "manager", "officer":
			return true
		}
	}

	return false
}

// knowsAll is whether somebody knows every capability a wish named, by
// exactly its id. See where a hire's capabilities are written down.
func knowsAll(can, wanted []string) bool {
	have := map[string]bool{}

	for _, one := range can {
		have[one] = true
	}

	for _, one := range wanted {
		if !have[one] {
			return false
		}
	}

	return true
}

func overlapOf(a, b []string) []string {
	in := map[string]bool{}

	for _, one := range b {
		in[one] = true
	}

	out := []string{}

	for _, one := range a {
		if in[one] {
			out = append(out, one)
		}
	}

	return out
}

/*
 * titleFor is what a person reads.
 *
 * The job's own title when the job already covers everything asked for — not
 * "Nurse specialist" for a nurse. The words that asked, when some of them are
 * more than the job: "Laravel security specialist" is a PHP developer whose
 * security is the point of hiring them, and calling them a PHP developer
 * loses the one thing that was asked for.
 */
func titleFor(words []string, job *store.Occupation) string {
	if len(uncovered(words, job)) == 0 {
		return job.Title
	}

	named := strings.Join(words, " ")

	return strings.ToUpper(named[:1]) + named[1:] + " specialist"
}

// uncovered is the words of a wish the job does not already answer to, in its
// title, its other names or what it needs.
func uncovered(words []string, job *store.Occupation) []string {
	out := []string{}

	for _, w := range words {
		if pointsFor(*job, stem(w)) == 0 {
			out = append(out, w)
		}
	}

	return out
}

/*
 * nameFor is the identifier. Letters a file name and a planner can both use,
 * which rules out anything but a–z — so a wish in Bulgarian takes its name
 * from the job rather than from the words.
 */
func nameFor(words []string, job *store.Occupation) string {
	candidate := strings.Join(words, "_") + "_specialist"

	if !validName.MatchString(candidate) {
		_, tail, found := strings.Cut(job.ID, ".")
		if !found {
			tail = job.ID
		}

		candidate = tail
	}

	if len(candidate) > 36 {
		candidate = strings.TrimRight(candidate[:36], "_")
	}

	return candidate
}

// unusedName adds a number until nobody on the roster has the name.
func unusedName(roster []Agent, name string) string {
	taken := map[string]bool{}

	for _, a := range roster {
		taken[a.Name] = true
	}

	if !taken[name] {
		return name
	}

	for i := 2; ; i++ {
		if next := fmt.Sprintf("%s_%d", name, i); !taken[next] {
			return next
		}
	}
}

/*
 * Dissolve lets go of everybody hired for one task, and says who.
 *
 * Their files go; their work does not — every step they did is still recorded
 * against their name, which is where anybody would look for it. Suspending
 * them instead would leave the roster a little more cluttered after every task
 * that needed a specialist, and a roster that grows on its own is one nobody
 * trusts.
 */
func Dissolve(root string, roster []Agent, task int64) ([]Agent, error) {
	gone := []Agent{}

	for _, a := range roster {
		if a.State != Temporary || a.HiredFor != task || a.BuiltIn {
			continue
		}

		err := os.Remove(filepath.Join(Folder(root), a.Name+".md"))
		if err != nil && !os.IsNotExist(err) {
			return gone, err
		}

		gone = append(gone, a)
	}

	if len(gone) > 0 {
		Forget()
	}

	return gone, nil
}

// roleWords name a kind of worker without the usual endings.
var roleWords = map[string]bool{
	"guard": true, "nurse": true, "doctor": true, "chef": true, "cook": true, "pilot": true, "clerk": true,
	"judge": true, "coach": true, "guide": true, "agent": true, "executive": true, "representative": true,
	"detective": true, "surgeon": true, "medic": true, "vet": true, "tutor": true, "artist": true,
	"architect": true, "analyst": true, "specialist": true, "consultant": true, "assistant": true,
}

/*
 * roleNoun is the last word of a wish of two words or more, when it names a
 * kind of worker — "tester" in "pen tester", "guard" in "security guard" —
 * and empty when it does not: "game development", "fix my roof".
 */
func roleNoun(words []string) string {
	if len(words) < 2 {
		return ""
	}

	w := strings.ToLower(words[len(words)-1])

	if roleWords[w] {
		return w
	}

	if len([]rune(w)) < 5 || strings.HasSuffix(w, "ment") || strings.HasSuffix(w, "ing") {
		return ""
	}

	for _, ending := range []string{"er", "or", "ist", "ian", "ant", "eer"} {
		if strings.HasSuffix(w, ending) {
			return w
		}
	}

	return ""
}

// calledA is whether a job is called by a word, in its title or any of its
// other names.
func calledA(job store.Occupation, head string) bool {
	if wordIn(strings.ToLower(job.Title), head) {
		return true
	}

	for _, alias := range job.Aliases {
		if wordIn(strings.ToLower(alias), head) {
			return true
		}
	}

	return false
}
