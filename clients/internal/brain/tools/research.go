package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/proof"
	"pn-scripts-assistant/internal/brain/research"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Research with sources: a question looked into on the web, written up as a
 * report whose every sentence says which source it came from, and then
 * checked against those sources.
 *
 * The model plans the searches and writes the report; the research package
 * decides which results are read, how far each can be trusted, and goes
 * through the finished report sentence by sentence. What the rules flag is
 * put to a model a second time as a fact checker, shown the passage of the
 * source and not the report, and told to judge from the passage alone.
 *
 * Reading the web is the one thing here that leaves the machine — the search
 * words, and a request for each page — so research is hidden whenever the
 * privacy setting keeps this machine to itself, like the search it is made
 * of. Checking somebody's own text against their own files leaves nothing.
 */

// ResearchBook is where finished reports are kept.
type ResearchBook interface {
	SaveResearch(r store.ResearchReport) (store.ResearchReport, error)
}

// Researching is what the research tools and the Research page share.
type Researching struct {
	Book ResearchBook

	// Search and Fetch reach the web. Nil Search is no web.
	Search func(ctx context.Context, query string) ([]SearchResult, error)
	Fetch  func(ctx context.Context, address string) (string, error)

	// Ask has the model write something, as for the other writing tools.
	Ask func(ctx context.Context, system, user string) (string, string, error)

	// Web says whether reaching the web is allowed now. Nil is allowed.
	Web func() bool

	// Now is the clock, replaceable in tests.
	Now func() time.Time
}

func (r Researching) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}

	return time.Now()
}

func (r Researching) webAllowed() bool { return r.Web == nil || r.Web() }

var (
	// ErrNoResearchModel is research asked for with no model to write it.
	ErrNoResearchModel = errors.New("no language model is available to write the report")

	// ErrNoWeb is research while the privacy setting keeps this machine to itself.
	ErrNoWeb = errors.New("research reads the web, and the privacy setting keeps this machine to itself; " +
		"allow the web in Control to use it")

	// ErrNothingRead is research that found nothing it could read.
	ErrNothingRead = errors.New("no source could be found or read for that question")
)

// Researched is a finished piece of research.
type Researched struct {
	Report  store.ResearchReport `json:"report"`
	Sources []research.Source    `json:"sources"`
	Claims  []research.Claim     `json:"claims"`
	Summary research.Summary     `json:"summary"`
	Queries []string             `json:"queries"`
}

func researchLanguage(lang, text string) string {
	if lang == "en" || lang == "bg" {
		return lang
	}

	return proof.Language(text)
}

/*
 * Research looks a question up, writes the report and checks it.
 *
 *  1. The model is asked what to search for; the question itself is always
 *     searched first, so a plan that cannot be read still finds something.
 *  2. The results are pooled and the best of them picked (research.Pick).
 *  3. The picked pages are read four at a time; a page that cannot be read
 *     is kept with its search snippet and marked as only partly read, and
 *     one with nothing in it is left out.
 *  4. The model writes the report from the numbered sources.
 *  5. The check goes through it; what it flags is put to the fact checker.
 */
func (r Researching) Research(ctx context.Context, question, depth, lang string) (Researched, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return Researched{}, errors.New("say what to research")
	}

	d, ok := research.DepthNamed(depth)
	if !ok {
		return Researched{}, errors.New("the depth must be quick or thorough")
	}

	if !r.webAllowed() || r.Search == nil {
		return Researched{}, ErrNoWeb
	}

	if r.Ask == nil {
		return Researched{}, ErrNoResearchModel
	}

	lang = researchLanguage(lang, question)

	// A model that cannot plan cannot write the report either: stop before
	// anything is searched, rather than send the searches for nothing.
	plan, _, err := r.Ask(ctx, research.PlanBrief(lang, d.Queries), question)
	if err != nil {
		return Researched{}, err
	}

	queries := research.ParsePlan(plan, question, d.Queries)

	var hits []research.Hit

	for _, q := range queries {
		found, err := r.Search(ctx, q)
		if err != nil {
			if ctx.Err() != nil {
				return Researched{}, ctx.Err()
			}

			// One search failing — a query refused for holding personal
			// details, an engine timing out — is not the whole research.
			continue
		}

		for _, f := range found {
			hits = append(hits, research.Hit{Title: f.Title, URL: f.URL, Snippet: f.Snippet})
		}
	}

	sources := r.read(ctx, research.Pick(hits, d.Sources), d.Chars)
	if len(sources) == 0 {
		return Researched{}, ErrNothingRead
	}

	body, model, err := r.Ask(ctx, research.ReportBrief(lang), research.SourcesBlock(question, sources))
	if err != nil {
		return Researched{}, err
	}

	body, claims, sum := research.Check(body, sources, lang)
	claims, sum = r.factCheck(ctx, claims, sources, sum)

	md := research.Render(question, body, lang, sources, claims, sum, r.now())

	srcJSON, _ := json.Marshal(sources)
	claimsJSON, _ := json.Marshal(claims)

	kept := store.ResearchReport{
		Question: question, Lang: lang, Depth: d.Name, Report: md, Model: model,
		Sources: srcJSON, Claims: claimsJSON,
		NSources: len(sources), NClaims: sum.Claims, NFlagged: sum.Flagged,
	}

	if r.Book != nil {
		if kept, err = r.Book.SaveResearch(kept); err != nil {
			return Researched{}, err
		}
	}

	return Researched{Report: kept, Sources: sources, Claims: claims, Summary: sum, Queries: queries}, nil
}

// read fetches the picked pages, four at a time, and numbers what was read.
func (r Researching) read(ctx context.Context, hits []research.Hit, chars int) []research.Source {
	type got struct {
		text    string
		partial bool
	}

	results := make([]got, len(hits))

	var wg sync.WaitGroup

	gate := make(chan struct{}, 4)

	for i, h := range hits {
		wg.Add(1)

		go func(i int, h research.Hit) {
			defer wg.Done()

			gate <- struct{}{}
			defer func() { <-gate }()

			text := ""

			if r.Fetch != nil {
				c, stop := context.WithTimeout(ctx, 30*time.Second)
				t, err := r.Fetch(c, h.URL)

				stop()

				if err == nil {
					text = strings.TrimSpace(t)
				}
			}

			if len(text) < 200 {
				results[i] = got{text: strings.TrimSpace(h.Snippet), partial: true}

				return
			}

			results[i] = got{text: text}
		}(i, h)
	}

	wg.Wait()

	now := r.now()
	read := now.Format("2006-01-02")

	var out []research.Source

	for i, h := range hits {
		g := results[i]
		if len(g.text) < 40 {
			continue
		}

		title := strings.TrimSpace(h.Title)
		if title == "" {
			title = h.URL
		}

		out = append(out, research.Source{
			N: len(out) + 1, Title: title, URL: h.URL, Grade: research.GradeURL(h.URL),
			Date: research.FindDate(g.text, now), Read: read, Partial: g.partial,
			Text: research.Cut(g.text, chars),
		})
	}

	return out
}

// maxJudged is how many flagged claims one report puts to the fact checker.
const maxJudged = 8

/*
 * factCheck puts the claims the rules flagged — those that cite a source —
 * to a model, one at a time, with the passage of each cited source. A claim
 * that cites nothing has nothing to be checked against and stays flagged.
 * No model, or no answer, leaves the rule's finding as it was.
 */
func (r Researching) factCheck(ctx context.Context, claims []research.Claim, sources []research.Source, sum research.Summary) ([]research.Claim, research.Summary) {
	if r.Ask == nil {
		return claims, sum
	}

	asked := 0

	for i := range claims {
		c := &claims[i]
		if c.Status == research.Supported || c.Status == research.Unsourced || c.Status == research.Unread ||
			len(c.Cites) == 0 || asked >= maxJudged {
			continue
		}

		if ctx.Err() != nil {
			break
		}

		asked++

		reply, _, err := r.Ask(ctx, research.JudgeBrief(), research.JudgeInput(*c, sources))
		if err != nil {
			continue
		}

		c.Verdict, c.Why = research.ParseVerdict(reply)
	}

	return claims, research.Recount(claims, sum.Removed)
}

// Checked is somebody's own text checked against sources they gave.
type Checked struct {
	Text    string            `json:"text"`
	Sources []research.Source `json:"sources"`
	Claims  []research.Claim  `json:"claims"`
	Summary research.Summary  `json:"summary"`
	Unread  []string          `json:"unread,omitempty"`
}

/*
 * CheckClaims checks a text that already cites its sources as [1], [2] … —
 * somebody's essay, an article, an earlier answer — against those sources,
 * given in order as web addresses or files. Nothing is kept.
 */
func (r Researching) CheckClaims(ctx context.Context, text string, refs []string, lang string) (Checked, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Checked{}, errors.New("give the text to check")
	}

	if len(refs) == 0 {
		return Checked{}, errors.New("give the sources the text cites, in the order of their numbers")
	}

	if len(refs) > 20 {
		return Checked{}, errors.New("at most 20 sources can be checked at once")
	}

	lang = researchLanguage(lang, text)
	now := r.now()

	var (
		sources []research.Source
		unread  []string
	)

	for i, ref := range refs {
		ref = strings.TrimSpace(ref)

		got, title, err := r.readRef(ctx, ref)
		if err != nil {
			unread = append(unread, fmt.Sprintf("[%d] %s: %v", i+1, ref, err))
			got = ""
		}

		sources = append(sources, research.Source{
			N: i + 1, Title: title, URL: ref, Grade: research.GradeURL(ref),
			Date: research.FindDate(got, now), Read: now.Format("2006-01-02"),
			Partial: err != nil, Text: research.Cut(got, 20000),
		})
	}

	fixed, claims, sum := research.Check(text, sources, lang)
	claims, sum = r.factCheck(ctx, claims, sources, sum)

	for i := range sources {
		sources[i].Text = ""
	}

	return Checked{Text: fixed, Sources: sources, Claims: claims, Summary: sum, Unread: unread}, nil
}

// plainKinds are files read as they are.
var plainKinds = map[string]bool{".txt": true, ".md": true, ".markdown": true, ".csv": true, ".html": true, ".htm": true}

func (r Researching) readRef(ctx context.Context, ref string) (string, string, error) {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		if !r.webAllowed() || r.Fetch == nil {
			return "", ref, ErrNoWeb
		}

		c, stop := context.WithTimeout(ctx, 30*time.Second)
		defer stop()

		text, err := r.Fetch(c, ref)

		return text, ref, err
	}

	if !filepath.IsAbs(ref) {
		return "", ref, errors.New("give a web address or an absolute path")
	}

	if err := GuardSensitive(ctx, ref); err != nil {
		return "", filepath.Base(ref), err
	}

	if plainKinds[strings.ToLower(filepath.Ext(ref))] {
		data, err := os.ReadFile(ref)
		if err != nil {
			return "", filepath.Base(ref), err
		}

		text := string(data)
		if strings.HasSuffix(strings.ToLower(ref), "htm") || strings.HasSuffix(strings.ToLower(ref), "html") {
			text = ReadableText(text)
		}

		return text, filepath.Base(ref), nil
	}

	args, _ := json.Marshal(map[string]string{"path": ref})
	text, err := ReadDocument{}.Execute(ctx, args)

	return text, filepath.Base(ref), err
}

// researchCues are the words that mean somebody wants something looked into
// properly. Not "find" or "search": those are web_search's, and a quick
// question should get a quick answer.
var researchCues = []string{
	"research", "investigat", "look into", "sources", "cite", "citation", "fact-check", "fact check",
	"report on", "literature", "evidence", "verify", "проучи", "проучване", "изследв", "източни",
	"цитат", "провери факт", "провери твърд", "доклад", "доказателств",
}

// ResearchTool looks a question up and writes a checked report.
type ResearchTool struct{ With Researching }

func (ResearchTool) Name() string   { return "research" }
func (ResearchTool) Risk() Risk     { return Safe }
func (ResearchTool) Cues() []string { return researchCues }

// LeavesTheMachine hides it with the other web tools when privacy says so.
func (ResearchTool) LeavesTheMachine() bool { return true }

func (ResearchTool) Description() string {
	return "Research a question on the web and write a report with numbered sources: it plans the searches, " +
		"reads the best sources (official and scholarly first, never one site over and over), writes the " +
		"report from them only, then checks every sentence against the source it cites and flags what the " +
		"sources do not say. Takes a minute or more. Keeps the report. Use it when somebody wants a question " +
		"looked into properly or wants sources; a quick fact needs only web_search."
}

func (ResearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {"type": "string", "description": "What to find out, as a full question"},
			"depth": {"type": "string", "enum": ["quick", "thorough"], "description": "quick: 4 sources; thorough: 8"},
			"language": {"type": "string", "enum": ["auto", "en", "bg"], "description": "The report's language"}
		},
		"required": ["question"]
	}`)
}

type researchArgs struct {
	Question string `json:"question"`
	Depth    string `json:"depth"`
	Language string `json:"language"`
}

func (ResearchTool) Summarize(raw json.RawMessage) string {
	var a researchArgs

	_ = json.Unmarshal(raw, &a)

	return "Research: " + a.Question
}

func (t ResearchTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a researchArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	done, err := t.With.Research(ctx, a.Question, a.Depth, a.Language)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	fmt.Fprintf(&b, "Research report %d is kept (Research page). %d sources read; %d of %d claims supported",
		done.Report.ID, len(done.Sources), done.Summary.Supported, done.Summary.Claims)

	if done.Summary.Flagged > 0 {
		fmt.Fprintf(&b, ", %d flagged — say which and why", done.Summary.Flagged)
	}

	b.WriteString(". Give the short answer with its source numbers, and do not add facts the report does not have.\n\n")
	b.WriteString(done.Report.Report)

	return b.String(), nil
}

// CheckClaimsTool checks a text against the sources it cites.
type CheckClaimsTool struct{ With Researching }

func (CheckClaimsTool) Name() string   { return "check_claims" }
func (CheckClaimsTool) Risk() Risk     { return Safe }
func (CheckClaimsTool) Cues() []string { return researchCues }

func (CheckClaimsTool) Description() string {
	return "Fact-check a text that cites its sources as [1], [2] … against those sources, given in order as " +
		"web addresses or absolute paths of files: says for every sentence whether its source supports it, " +
		"which figures are not in the source, which sentences cite nothing, and removes citations to sources " +
		"that do not exist. Changes nothing and keeps nothing."
}

func (CheckClaimsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"text": {"type": "string", "description": "The text to check, with its [n] citations"},
			"sources": {"type": "array", "items": {"type": "string"}, "description": "Source 1, 2, … as web addresses or absolute file paths"},
			"language": {"type": "string", "enum": ["auto", "en", "bg"]}
		},
		"required": ["text", "sources"]
	}`)
}

type checkClaimsArgs struct {
	Text     string   `json:"text"`
	Sources  []string `json:"sources"`
	Language string   `json:"language"`
}

func (CheckClaimsTool) Summarize(raw json.RawMessage) string {
	var a checkClaimsArgs

	_ = json.Unmarshal(raw, &a)

	return fmt.Sprintf("Check a text's claims against %d sources", len(a.Sources))
}

// Reaches names the hosts it would read, for Control's record.
func (CheckClaimsTool) Reaches(raw json.RawMessage) string {
	var a checkClaimsArgs

	_ = json.Unmarshal(raw, &a)

	var hosts []string

	for _, s := range a.Sources {
		if h := hostOf(s); h != "" {
			hosts = append(hosts, h)
		}
	}

	return strings.Join(hosts, ", ")
}

func (t CheckClaimsTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a checkClaimsArgs

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	done, err := t.With.CheckClaims(ctx, a.Text, a.Sources, a.Language)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%d claims checked: %d supported, %d flagged.", done.Summary.Claims, done.Summary.Supported, done.Summary.Flagged)

	if done.Summary.Removed > 0 {
		fmt.Fprintf(&b, " %d citations pointed to no source and were removed.", done.Summary.Removed)
	}

	b.WriteString("\n")

	for _, u := range done.Unread {
		fmt.Fprintf(&b, "Could not read %s\n", u)
	}

	for _, c := range done.Claims {
		mark := "OK"
		if c.Flagged() {
			mark = "CHECK"
		}

		note := c.Note
		if c.Verdict != "" {
			note = strings.ReplaceAll(c.Verdict, "_", " ")
			if c.Why != "" {
				note += ": " + c.Why
			}
		}

		if note != "" {
			note = " — " + note
		}

		fmt.Fprintf(&b, "\n%s: %s%s", mark, c.Text, note)
	}

	return b.String(), nil
}
