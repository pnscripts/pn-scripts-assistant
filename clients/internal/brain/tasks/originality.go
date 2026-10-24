package tasks

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Checks before a result is called finished — only the ones it needs.
 *
 * Every produced result (a game, a logo, a jingle, a story, a business
 * strategy, a contract draft) is checked for originality and meaning by the
 * specialists whose field it touches, side by side, and their findings are
 * part of the final report (knowledge/guides/originality.md, how-i-work.md).
 * Only if needed (owner 2026-10-24): no sound specialist for a result without
 * sound, no trademark search for an internal draft, one light check for a
 * small change. Why each check was or was not used is written on the first
 * check, so it can be read in the task.
 */

// OriginalityMarker starts the instruction of every check step.
const OriginalityMarker = "Originality check"

type aspect struct {
	name       string
	specialist string
	cues       []string
	check      string
}

var aspects = []aspect{
	{"name and trademarks", "ip_specialist",
		[]string{"name for", "brand", "logo", "trademark", "product name", "title for",
			"име за", "марка", "лого", "заглавие за"},
		"search the name in trademark registers and the stores where it will appear; no existing product's name, logo or trade dress"},
	{"look", "visual_reviewer",
		[]string{"logo", "character", "mascot", "icon", "illustration", "artwork", "art", "design", "game", "comic", "animation",
			"лого", "герой", "персонаж", "талисман", "икон", "илюстрац", "рисун", "дизайн", "игра", "комикс", "анимац"},
		"compare characters, logos and style with the closest known works; nothing made to look like a protected one"},
	{"sound", "sound_reviewer",
		[]string{"music", "song", "melody", "jingle", "sound", "audio", "soundtrack",
			"музик", "песен", "мелоди", "джингъл", "звук", "аудио"},
		"compare melodies, jingles and sound logos with known music; nothing played \"by ear\" from a known tune"},
	{"game rules", "game_design_reviewer",
		[]string{"game", "level", "puzzle", "игра", "ниво", "пъзел"},
		"say in one sentence what only this game does and compare its core rule with the three closest famous titles"},
	{"text and meaning", "copy_reviewer",
		[]string{"story", "novel", "poem", "slogan", "text", "article", "post", "copy", "letter", "strategy", "plan", "report", "document",
			"истори", "роман", "стихотвор", "слоган", "текст", "статия", "публикация", "писмо", "стратеги", "план", "отчет", "документ"},
		"check the text against known works and that every part says something real for this reader"},
	{"money", "finance_analyst",
		[]string{"budget", "price", "pricing", "revenue", "cost", "profit", "invest", "loan", "forecast",
			"бюджет", "цена", "приход", "разход", "печалба", "инвест", "кредит", "прогноз"},
		"check every figure, assumption and calculation"},
	{"tax and accounting", "accountant",
		[]string{"tax", "vat", "invoice", "accounting", "данък", "ддс", "фактур", "счетовод"},
		"check tax and accounting statements against the current rules of the country named"},
	{"law", "legal_advisor",
		[]string{"contract", "terms", "privacy policy", "gdpr", "licence", "license", "legal",
			"договор", "условия", "поверителност", "лиценз", "правн", "закон"},
		"check legal statements, licences and obligations; say what needs a qualified lawyer"},
	{"business", "business_strategist",
		[]string{"business", "strategy", "market", "competitor", "startup", "бизнес", "стратеги", "пазар", "конкурент"},
		"check the strategy is the business's own, grounded in its market and not a copy of a known company's"},
}

// draftCues mean the result is for the owner's eyes only: one light check.
var draftCues = []string{"draft", "sketch", "internal", "for myself", "quick", "чернова", "скица", "вътрешн", "за мен", "набързо"}

func mentions(text string, cues []string) string {
	for _, cue := range cues {
		if strings.Contains(text, cue) {
			return cue
		}
	}

	return ""
}

// checksFor is which specialists a request needs and why each other one is
// not needed.
func checksFor(request string) (use []aspect, why []string) {
	text := strings.ToLower(request)

	for _, a := range aspects {
		if cue := mentions(text, a.cues); cue != "" {
			use = append(use, a)
			why = append(why, a.name+": yes (\""+cue+"\")")
		} else {
			why = append(why, a.name+": not needed")
		}
	}

	if len(use) > 1 && mentions(text, draftCues) != "" {
		// A draft gets one light check: the first aspect it has.
		use = use[:1]
		why = append(why, "a draft: one light check only")
	}

	return use, why
}

// withOriginality ends a plan that produces something with its checks.
func withOriginality(request string, p Plan) Plan {
	for _, s := range p.Steps {
		if strings.HasPrefix(s.Instruction, OriginalityMarker) {
			return p
		}
	}

	use, why := checksFor(request)
	if len(use) == 0 {
		return p
	}

	for i, a := range use {
		instruction := OriginalityMarker + " (" + a.name + "): " + a.check +
			". Generated by code is not an exception. " + verdictFormat

		if i == 0 {
			instruction += " Checks for this result — " + strings.Join(why, "; ") + "."
		}

		p.Steps = append(p.Steps, store.TaskStep{
			Kind:        store.StepCheck,
			Instruction: instruction,
			Assignee:    a.specialist,
			DoneWhen:    "the findings name what was compared and what, if anything, must change",
			// Side by side: every check after the first runs with it.
			Together: i > 0,
		})
	}

	return p
}

// OriginalitySection heads the checks in a task's final report.
const OriginalitySection = "Originality and meaning:"

// originalityLine is how one check step reads in the final report, or "" for
// a step that is not a check.
func originalityLine(s store.TaskStep) string {
	if !strings.HasPrefix(s.Instruction, OriginalityMarker) {
		return ""
	}

	what := "check"

	if open := strings.Index(s.Instruction, "("); open >= 0 {
		if end := strings.Index(s.Instruction[open:], ")"); end > 0 {
			what = s.Instruction[open+1 : open+end]
		}
	}

	switch {
	case s.State == store.StepDone && s.Verdict == store.Unmet:
		return "• " + what + ": found wrong on review — " + trimTo(s.Answer, 300)
	case s.State == store.StepDone && s.Answer != "":
		return "• " + what + ": " + trimTo(s.Answer, 300)
	case s.State == store.StepDone:
		return "• " + what + ": nothing found"
	case s.State == store.StepSkipped:
		return "• " + what + ": skipped"
	default:
		return "• " + what + ": not done"
	}
}

/*
 * verdictFormat is how a check answers, so that what it found can be acted
 * on without anybody reading it first: OK, or CHANGE and what.
 */
const verdictFormat = "Begin the answer with \"OK:\" when nothing must change, or \"CHANGE:\" followed by " +
	"exactly what is too close or empty and what to change it to."

// mustChange is whether a check's answer asks for a change.
func mustChange(answer string) bool {
	a := strings.ToUpper(strings.TrimLeft(answer, " *_#>\n\t"))

	return strings.HasPrefix(a, "CHANGE")
}

// aspectOf is the aspect named in a check's instruction.
func aspectOf(instruction string) string {
	if open := strings.Index(instruction, "("); open >= 0 {
		if end := strings.Index(instruction[open:], ")"); end > 0 {
			return strings.TrimSuffix(instruction[open+1:open+end], ", again")
		}
	}

	return ""
}

/*
 * afterCheck is what follows a finished check, automatically (owner
 * 2026-10-24: "всичко трябва да е автоматично!").
 *
 * When the job made the thing itself and a check says it must change, the
 * one who made it changes it and the same specialist checks it again — once;
 * what the second check still finds goes into the report as it is, rather
 * than round and round. A user's own works (an audit) are never changed:
 * they get the review of the specialists their findings need, and the report.
 */
func (c *Conductor) afterCheck(task *store.Task, step *store.TaskStep) error {
	if strings.HasPrefix(step.Instruction, AuditMarker) {
		return c.reviewAudit(task)
	}

	if !strings.HasPrefix(step.Instruction, OriginalityMarker) || !mustChange(step.Answer) {
		return nil
	}

	aspect := aspectOf(step.Instruction)
	if aspect == "" || strings.Contains(step.Instruction, "("+aspect+", again)") {
		return nil
	}

	steps, err := c.DB.Steps(task.ID)
	if err != nil {
		return err
	}

	// Who made it: the last step of the work itself before the checks.
	var maker *store.TaskStep

	for i := range steps {
		s := &steps[i]

		if strings.HasPrefix(s.Instruction, AuditMarker) {
			return nil // the user's works: reported, never changed
		}

		if !strings.HasPrefix(s.Instruction, OriginalityMarker) && !strings.HasPrefix(s.Instruction, FixMarker) &&
			s.State == store.StepDone {
			maker = s
		}
	}

	if maker == nil {
		return nil
	}

	again := OriginalityMarker + " (" + aspect + ", again)" + step.Instruction[strings.Index(step.Instruction, ")")+1:]

	return c.DB.AddSteps(task.ID, []store.TaskStep{
		{
			Kind:        store.StepDo,
			Changes:     true,
			Assignee:    maker.Assignee,
			Instruction: FixMarker + " (" + aspect + "): " + trimTo(step.Answer, 600) + " Change only what this names; keep what the work is for.",
			DoneWhen:    "the work no longer has what the check found",
		},
		{
			Kind:        store.StepCheck,
			Assignee:    step.Assignee,
			Instruction: again,
			DoneWhen:    step.DoneWhen,
		},
	})
}

// FixMarker starts the instruction of a step that fixes a check's finding.
const FixMarker = "Fix what the originality check found"

// AuditMarker starts the instruction of a catalogue audit.
const AuditMarker = "Originality audit"

var (
	auditCue  = regexp.MustCompile(`(?i)\b(audit|check|review|scan)|одит|провер|прегледа`)
	originCue = regexp.MustCompile(`(?i)\b(originality|original|unique|uniqueness|copied|plagiari[sz]|trademark)|оригинал|уникал|копира|плагиат|търговск`)
	// Asked to make something and check it: that is a job with checks, not
	// an audit.
	makeCue = regexp.MustCompile(`(?i)\b(make|create|build|write|compose|design|draw)\b|направи|създай|напиши|нарисувай|композирай|измисли`)
	pathCue = regexp.MustCompile(`(?:~|\.{0,2}/|[A-Za-z]:\\)[^\s"'„“”]+`)
)

// auditPlan is the plan for an originality audit of the user's own works:
// one audit across everything, side by side, then the specialists only for
// the aspects that found something (added when the audit has run).
func auditPlan(request string) (Plan, bool) {
	if !auditCue.MatchString(request) || !originCue.MatchString(request) || makeCue.MatchString(request) {
		return Plan{}, false
	}

	folder := "the folder or catalogue the user named"
	if p := pathCue.FindString(request); p != "" {
		folder = p
	}

	return Plan{
		Name: nameFor(request),
		Steps: []store.TaskStep{{
			Kind:     store.StepDo,
			Assignee: "ip_specialist",
			Instruction: AuditMarker + ": run audit_originality on " + folder +
				" (with the reference pictures the user gave, if any). Report the summary and the findings with high and medium risk. " +
				"These are the user's own works: change nothing.",
			DoneWhen: "the audit report is saved and its path is in the answer",
		}},
	}, true
}

// auditSpecialists are who reviews which aspect of an audit's findings.
var auditSpecialists = map[string]string{
	"name and trademarks": "ip_specialist",
	"look":                "visual_reviewer",
	"sound":               "sound_reviewer",
	"game rules":          "game_design_reviewer",
	"text and meaning":    "copy_reviewer",
}

// latestReport is the newest audit sheet in the brain's reports folder.
func latestReport() string {
	sheets, _ := filepath.Glob(filepath.Join(tools.Root, "reports", "originality", "*.csv"))
	if len(sheets) == 0 {
		return ""
	}

	sort.Slice(sheets, func(i, j int) bool {
		a, _ := os.Stat(sheets[i])
		b, _ := os.Stat(sheets[j])

		return a != nil && b != nil && a.ModTime().After(b.ModTime())
	})

	return sheets[0]
}

// reviewAudit adds one review per aspect with high or medium findings, side
// by side; an aspect with nothing serious gets no specialist.
func (c *Conductor) reviewAudit(task *store.Task) error {
	sheet := latestReport()
	if sheet == "" {
		return nil
	}

	f, err := os.Open(sheet)
	if err != nil {
		return err
	}
	defer f.Close()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return err
	}

	count := map[string]int{}

	for _, r := range rows {
		if len(r) >= 4 && (r[3] == "high" || r[3] == "medium") {
			count[r[1]]++
		}
	}

	var reviews []store.TaskStep

	for _, a := range aspects {
		who, ok := auditSpecialists[a.name]
		if !ok || count[a.name] == 0 {
			continue
		}

		reviews = append(reviews, store.TaskStep{
			Kind:     store.StepCheck,
			Assignee: who,
			Instruction: OriginalityMarker + " (" + a.name + "): review the " + a.name + " findings marked high or medium in " +
				sheet + " — " + a.check + ". For each, say whether it stands and what the owner should change. " +
				"These are the user's own works: recommend, change nothing. " + verdictFormat,
			DoneWhen: "every high and medium " + a.name + " finding is confirmed or dismissed with a reason",
			Together: len(reviews) > 0,
		})
	}

	return c.DB.AddSteps(task.ID, reviews)
}

// makesSomething is whether a request produces something new — what gets a
// folder of its own in goal mode — rather than working on things the user
// already has, which a request naming a path does.
func makesSomething(request string) bool {
	if pathCue.MatchString(request) {
		return false
	}

	if makeCue.MatchString(request) {
		return true
	}

	use, _ := checksFor(request)

	return len(use) > 0 && !auditCue.MatchString(request)
}
