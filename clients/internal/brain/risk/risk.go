/*
 * Package risk is how serious a piece of work is, in four words.
 *
 * Low, medium, high, critical. Separate from tools.Risk, which is the gate and
 * stays exactly two values: a tool either changes something or it does not,
 * and that is what decides whether anybody is asked. This is the other
 * question — how much it matters if it goes wrong — and it only ever adds
 * caution on top of the gate, never removes any.
 *
 * What each level does, which is deliberately little:
 *
 *   low, medium  shown, and nothing else. Most work is one of these.
 *   high         shown, named in the approval question, and listed in the
 *                task's account of itself when it ran without anybody asked.
 *   critical     all of that, and it asks even where a standing grant would
 *                have let it through — "you may send email" is not "you may
 *                move money". Except on never stop, never refuse, which its
 *                owner chose as nothing asking at all. There it is recorded
 *                instead, and the record says it would have been asked.
 *
 * A package of its own, with nothing imported, because the tools, the gate,
 * the agent loop, the task conductor and the store all need the same four
 * words, and none of them should need each other to get them.
 */
package risk

import (
	"strings"
	"unicode"
)

// Level is one of the four.
type Level string

const (
	Low      Level = "low"
	Medium   Level = "medium"
	High     Level = "high"
	Critical Level = "critical"
)

// All is the four, least serious first.
func All() []Level { return []Level{Low, Medium, High, Critical} }

func (l Level) rank() int {
	switch l {
	case Medium:
		return 1
	case High:
		return 2
	case Critical:
		return 3
	default:
		return 0
	}
}

/*
 * Parse reads a level written down somewhere.
 *
 * Empty and anything unrecognised are low. Levels only add caution, so a
 * missing one must not invent some — a row written before there were levels
 * is exactly as serious as it was the day before they existed.
 */
func Parse(written string) Level {
	switch l := Level(strings.ToLower(strings.TrimSpace(written))); l {
	case Medium, High, Critical:
		return l
	default:
		return Low
	}
}

// Max is the most serious of them.
func Max(levels ...Level) Level {
	out := Low

	for _, l := range levels {
		if l.rank() > out.rank() {
			out = l
		}
	}

	return out
}

// AtLeast reports whether this is as serious as other, or more.
func (l Level) AtLeast(other Level) bool { return l.rank() >= other.rank() }

// Below is one step less serious, and low stays low.
func (l Level) Below() Level {
	switch l {
	case Critical:
		return High
	case High:
		return Medium
	default:
		return Low
	}
}

// Title is the level as a person reads it at the start of a sentence.
func (l Level) Title() string {
	switch l {
	case Medium:
		return "Medium"
	case High:
		return "High"
	case Critical:
		return "Critical"
	default:
		return "Low"
	}
}

/*
 * cues are the words that make a piece of work more serious than its shape.
 *
 * Matched at the start of a word, so "delet" is delete, deleted and deleting,
 * and never the middle of one — "prepay" is not "pay". Both languages this
 * assistant is spoken to in, because a Bulgarian instruction to pay somebody
 * is not less serious for being in Bulgarian.
 *
 * This reads an instruction a model wrote, and it is a guess, and it is
 * written to guess high: calling a step critical that was not costs one
 * question somebody answers in a second, and the other mistake costs whatever
 * the step did. It is also not the only line of defence. The tool actually
 * called is weighed on its own arguments at the moment it runs, which is the
 * more reliable of the two — this one decides what the plan looks like before
 * anything has happened.
 */
var cues = []struct {
	level Level
	words []string
}{
	{Critical, []string{
		// Money leaving.
		"pay ", "pay for", "payment", "transfer money", "transfer funds", "wire money",
		"send money", "buy ", "buying ", "purchase", "place an order", "order online",
		"bank account", "bank transfer", "credit card", "invoice payment",
		"tax return", "file taxes", "file the tax", "vat return",
		"плати", "плащане", "преведи пари", "банков превод", "купи", "купуване",
		"поръчай", "данъчна декларация", "кредитна карта",

		// Health and law, where the decision is somebody's own to make.
		"prescri", "dosage", "medication", "treatment plan", "medical decision",
		"legal advice", "sign the contract", "sign a contract", "lawsuit", "court filing",
		"рецепт", "дозировка", "лекарств", "лечение", "правен съвет", "подпиши договор",
		"съдебн",

		// Things that cannot be undone at all.
		"wipe ", "format the disk", "format disk", "drop database", "drop table",
		"delete everything", "delete all", "factory reset",
		"изтрий всичко", "форматирай",
	}},
	{High, []string{
		"deploy", "production", "database migration", "migrate the database", "dns ",
		"delet", "remov", "uninstall", "publish", "post public", "go live",
		"password", "credential", "account setting", "firewall", "sudo ",
		"force push", "push --force", "shut down", "shutdown", "reboot",
		"изтрий", "премахни", "деинсталирай", "публикувай", "парол", "продукционн",
		"рестартирай",
	}},
	{Medium, []string{
		"send", "email", "e-mail", "message", "reply", "invite", "book ", "schedule",
		"install", "commit", "push", "edit", "change", "update", "move ", "rename",
		"изпрати", "имейл", "съобщение", "отговори", "покани", "запази час",
		"инсталирай", "промени", "редактирай", "премести", "преименувай",
	}},
}

/*
 * InWords is how serious an instruction sounds, and the words that said so.
 *
 * The words come back so the interface can say why — "critical: pay" is
 * something a person can disagree with, and a bare "critical" is not.
 */
func InWords(text string) (Level, string) {
	padded := " " + normalise(text) + " "

	for _, group := range cues {
		for _, cue := range group.words {
			if strings.Contains(padded, " "+cue) {
				return group.level, strings.TrimSpace(cue)
			}
		}
	}

	return Low, ""
}

// normalise lowers the text and turns everything but letters, digits and the
// two marks commands are written with into single spaces, so a cue matches
// "Pay," and "pay." the same as "pay".
func normalise(text string) string {
	var b strings.Builder

	space := false

	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)

			space = false

			continue
		}

		if !space {
			b.WriteRune(' ')

			space = true
		}
	}

	return strings.TrimSpace(b.String())
}
