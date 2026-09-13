/*
 * Package occupations is the world of work, as against this program's staff.
 *
 * The difference is the point of it. A job definition describes something
 * whether or not anybody here does it — a tax accountant exists as an idea
 * before there is an agent holding that job, and describing one costs a row
 * rather than a running model. That is what makes "thousands of jobs, dozens
 * of agents" an honest claim rather than a boast: the thousands are a table.
 *
 * What ships built in is a seed of about a hundred, weighted towards the work
 * actually done here, and it is enough to run on with nothing downloaded. The
 * rest arrives by importing a real classification, because a hand-typed list
 * of occupations is a list of what one person had heard of — which is exactly
 * the mistake the model catalogue already made and had to undo.
 */
package occupations

import (
	"strings"
	"unicode"

	"pn-scripts-assistant/internal/brain/store"
)

/*
 * The categories, in the order somebody reads them.
 *
 * Broad on purpose. A taxonomy with sixty top-level headings is a taxonomy
 * nobody can hold in their head, and the fine distinctions belong in the job
 * itself rather than in the shelf it sits on.
 */
const (
	Leadership  = "leadership"
	Software    = "software"
	AI          = "ai"
	Data        = "data"
	Security    = "security"
	Safety      = "safety"
	Product     = "product"
	Design      = "design"
	Marketing   = "marketing"
	Sales       = "sales"
	Finance     = "finance"
	Legal       = "legal"
	People      = "people"
	Operations  = "operations"
	Support     = "support"
	Science     = "science"
	Engineering = "engineering"
	Media       = "media"
	Education   = "education"
	Health      = "health"
	Hospitality = "hospitality"
	Trades      = "trades"
	Land        = "land"
	Government  = "government"
	Property    = "property"
	Transport   = "transport"
	General     = "general"
)

// Shelf is one category as somebody reads it.
type Shelf struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Categories is the reading order, which is roughly "who decides, who builds,
// who sells, who keeps the books, then the rest of the world".
func Categories() []Shelf {
	return []Shelf{
		{Leadership, "Leadership"},
		{Software, "Software"},
		{AI, "AI and machine learning"},
		{Data, "Data"},
		{Security, "Security"},
		{Safety, "Safety and emergencies"},
		{Product, "Product"},
		{Design, "Design"},
		{Marketing, "Marketing"},
		{Sales, "Sales"},
		{Finance, "Finance"},
		{Legal, "Legal"},
		{People, "People"},
		{Operations, "Operations"},
		{Support, "Support"},
		{Science, "Science"},
		{Engineering, "Engineering"},
		{Media, "Media and writing"},
		{Education, "Education"},
		{Health, "Health"},
		{Hospitality, "Hospitality"},
		{Trades, "Trades"},
		{Land, "Land and growing"},
		{Government, "Government"},
		{Property, "Property"},
		{Transport, "Transport"},
		{General, "General"},
	}
}

// TitleOf is the readable name of a category, or the category itself when it
// is one somebody added.
func TitleOf(category string) string {
	for _, shelf := range Categories() {
		if shelf.ID == category {
			return shelf.Title
		}
	}

	return category
}

/*
 * Ensure puts the built-in seed into a brain that has not got it.
 *
 * Run every time rather than once, because it is a no-op when nothing has
 * changed — the writer compares before it writes — and because that is what
 * makes a job added in a later version of the program simply appear. Nothing
 * here can overwrite a job somebody edited: what shipped loses to what was
 * written by hand, in the store, by rank.
 */
func Ensure(db *store.DB) (store.Imported, error) {
	jobs, capabilities := Seed()

	return db.Import(jobs, capabilities)
}

/*
 * MakeID turns a title into an identifier, for the importers.
 *
 * Lower case, letters and digits, underscores between words. Non-English
 * letters are kept rather than stripped: an id built from a Bulgarian label
 * should still be that label, not a row of underscores. Ids from an import
 * are prefixed by their source elsewhere, so two classifications naming the
 * same job differently cannot collide into one row by accident.
 */
func MakeID(text string) string {
	var b strings.Builder

	lastWasBreak := true

	for _, r := range strings.ToLower(strings.TrimSpace(text)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastWasBreak = false
		case !lastWasBreak:
			b.WriteRune('_')
			lastWasBreak = true
		}
	}

	return strings.Trim(b.String(), "_")
}
