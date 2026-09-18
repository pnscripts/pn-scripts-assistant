package orchestrator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

/*
 * Which engine a game is made with, chosen rather than asked.
 *
 * Not "3D means Unity": the request says what it needs — in a browser, in
 * three dimensions, on a phone — and each engine says what it can really do
 * on this machine: installed and licensed or not, whether a project can be
 * made, whether it can be photographed, and what has already been proven
 * here by running it. The one that meets the request and can be checked
 * best is chosen, with why; the owner is asked only when nothing fits, or
 * when the request names something this machine cannot do.
 */

// EngineOption is one engine as a candidate for a project.
type EngineOption struct {
	Package string `json:"package"`
	Engine  string `json:"engine"`
	Title   string `json:"title"`

	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`

	// Licence is "", "active", "inactive" or "unknown".
	Licence string `json:"licence,omitempty"`

	// Installable is whether it can be installed from here, and Size how
	// big that would be.
	Installable bool   `json:"installable"`
	Size        string `json:"size,omitempty"`

	CanCreate bool `json:"can_create"`
	Picture   bool `json:"picture"`

	Platforms []string `json:"platforms"`
	Dims      []string `json:"dims"`
	Language  string   `json:"language"`
	HighEnd   bool     `json:"high_end,omitempty"`

	// Proven is what has been done with it on this machine and worked.
	Proven []string `json:"proven,omitempty"`

	// Why is why it cannot be used, when it cannot.
	Why string `json:"why,omitempty"`
}

// Usable is whether a new project can be made with it here, now or after an
// install that needs no licence.
func (e EngineOption) Usable() bool {
	return e.Why == "" && e.CanCreate && (e.Installed || e.Installable) && e.Licence != "inactive"
}

// Wants is what a request asks of an engine, read from its words.
type Wants struct {
	Web, Desktop, Mobile bool
	TwoD, ThreeD         bool
	HighEnd              bool
}

var (
	webWords     = regexp.MustCompile(`(?i)\b(browser|web|website|online|html5?|in a page|webgl)\b`)
	mobileWords  = regexp.MustCompile(`(?i)\b(mobile|phone|android|ios|iphone|tablet)\b`)
	desktopWords = regexp.MustCompile(`(?i)\b(desktop|pc|computer|windows|linux|mac|steam|native)\b`)
	threeWords   = regexp.MustCompile(`(?i)\b(3d|3-d|three[- ]dimensional|first[- ]person|fps|third[- ]person|racing|open[- ]world|flight|voxel|minecraft)\b`)
	twoWords     = regexp.MustCompile(`(?i)\b(2d|2-d|platformer|puzzle|tetris|snake|pong|breakout|arkanoid|pac-?man|card game|match[- ]3|shoot[- ]?'?em[- ]?up|side[- ]scroller|top[- ]down|pixel)\b`)
	highWords    = regexp.MustCompile(`(?i)\b(aaa|photo-?realistic|high[- ]end|cinematic|nanite|unreal quality)\b`)
)

// Read is what a sentence asks of an engine.
func Read(request string) Wants {
	return Wants{
		Web: webWords.MatchString(request), Mobile: mobileWords.MatchString(request),
		Desktop: desktopWords.MatchString(request),
		ThreeD:  threeWords.MatchString(request), TwoD: twoWords.MatchString(request),
		HighEnd: highWords.MatchString(request),
	}
}

func (w Wants) String() string {
	var parts []string

	for _, p := range []struct {
		on   bool
		what string
	}{{w.Web, "in a browser"}, {w.Desktop, "on the desktop"}, {w.Mobile, "on a phone"},
		{w.TwoD, "2D"}, {w.ThreeD, "3D"}, {w.HighEnd, "high-end graphics"}} {
		if p.on {
			parts = append(parts, p.what)
		}
	}

	if len(parts) == 0 {
		return "nothing particular about where it runs or how it looks"
	}

	return strings.Join(parts, ", ")
}

// EngineChoice is the engine chosen, the others and why.
type EngineChoice struct {
	Chosen   *EngineOption  `json:"chosen,omitempty"`
	Others   []EngineOption `json:"others,omitempty"`
	Rejected []Rejection    `json:"rejected,omitempty"`
	Reasons  []string       `json:"reasons,omitempty"`

	// Ask is the question for the owner, when nothing can be chosen safely.
	Ask string `json:"ask,omitempty"`
}

/*
 * ChooseEngine is the engine for a request among the options.
 *
 * The score is laid out so that each part can be said: fitting what was
 * asked — where it runs, 2D or 3D, high-end — first and heaviest; then how
 * ready it is here; then how well the result can be checked, since a game
 * that cannot be run and photographed cannot be shown to be finished; then
 * how familiar its language is to the model writing it; then what has been
 * proven here before.
 */
func ChooseEngine(options []EngineOption, request string, strongCoder bool) EngineChoice {
	w := Read(request)

	var (
		c      EngineChoice
		usable []EngineOption
	)

	for _, o := range options {
		switch {
		case !o.Usable():
			why := o.Why

			switch {
			case why != "":
			case !o.CanCreate:
				why = "this program cannot make a new project with it"
			case o.Licence == "inactive":
				why = "installed, and not licensed to work unattended"
			default:
				why = "not here, and not installable from here"
			}

			c.Rejected = append(c.Rejected, Rejection{ID: o.Engine, Title: o.Title, Why: why})
		case w.Web && !has(o.Platforms, "web"):
			c.Rejected = append(c.Rejected, Rejection{ID: o.Engine, Title: o.Title, Why: "it does not make games that run in a browser"})
		case w.Mobile && !has(o.Platforms, "mobile"):
			c.Rejected = append(c.Rejected, Rejection{ID: o.Engine, Title: o.Title, Why: "no phone build can be checked here"})
		case w.ThreeD && !has(o.Dims, "3d"):
			c.Rejected = append(c.Rejected, Rejection{ID: o.Engine, Title: o.Title, Why: "it is for 2D games"})
		default:
			usable = append(usable, o)
		}
	}

	if len(usable) == 0 {
		c.Ask = "Nothing on this machine can make this as asked (" + w.String() + "): " + summary(c.Rejected)

		return c
	}

	score := func(o EngineOption) (int, []string) {
		total := 0

		var why []string

		if w.Web && has(o.Platforms, "web") {
			total += 30
			why = append(why, "it runs in a browser, as asked")
		}

		if w.Desktop && has(o.Platforms, "desktop") {
			total += 20
			why = append(why, "it makes desktop games, as asked")
		}

		if w.ThreeD && has(o.Dims, "3d") {
			total += 20
			why = append(why, "it does 3D")
		}

		if w.TwoD && has(o.Dims, "2d") {
			total += 10
			why = append(why, "it does 2D well")
		}

		if w.HighEnd && o.HighEnd {
			total += 25
			why = append(why, "it is built for high-end graphics")
		}

		switch {
		case o.Installed:
			total += 12
			why = append(why, fmt.Sprintf("%s %s is installed here", o.Title, o.Version))
		case o.Installable:
			total += 4
			why = append(why, "it can be installed ("+orElse(o.Size, "size not known")+")")
		}

		if o.Picture {
			total += 10
			why = append(why, "a running game can be photographed here, so finished can be shown")
		}

		switch o.Language {
		case "javascript", "csharp":
			total += 6
		case "gdscript":
			if strongCoder {
				total += 6
			} else {
				total += 3
			}
		default:
			total += 2
		}

		if n := len(o.Proven); n > 0 {
			total += min(8, 2*n)
			why = append(why, "already proven here: "+strings.Join(o.Proven, ", "))
		}

		return total, why
	}

	sort.SliceStable(usable, func(i, j int) bool {
		a, _ := score(usable[i])
		b, _ := score(usable[j])

		if a != b {
			return a > b
		}

		return usable[i].Engine < usable[j].Engine
	})

	chosen := usable[0]
	_, c.Reasons = score(chosen)
	c.Chosen = &chosen
	c.Others = usable[1:]

	c.Reasons = append([]string{"asked for: " + w.String()}, c.Reasons...)

	if w.HighEnd && !chosen.HighEnd {
		c.Reasons = append(c.Reasons, "nothing usable here is built for high-end graphics, so this is the nearest — not the same")
	}

	return c
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}

	return false
}
