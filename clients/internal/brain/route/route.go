/*
 * Package route picks the model each expert works with, from what is really
 * available on this machine at the moment of asking.
 *
 * A role says what its work needs — how much reasoning, whether it writes
 * code, whether it has to see pictures, how long its documents are, how fast
 * an answer must come, whether nothing may leave the computer. The candidates
 * are the models Ollama has here, with what measuring them showed, plus cloud
 * models only when their owner switched that provider on. Pick is a pure
 * function of the two, so the same machine gives the same answer, a model
 * added or removed changes it at the next call, and a model that keeps
 * failing falls down the order by itself.
 *
 * Every choice carries its reason and the fallbacks after it, because "why
 * this model" is a question its owner will ask and the answer must be on the
 * screen, not reconstructed from a log.
 */
package route

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Needs is what an expert's work asks of a model. Levels run 0 (does not
// matter) to 3 (the most there is).
type Needs struct {
	Reasoning   int  `json:"reasoning,omitempty"`
	Coding      int  `json:"coding,omitempty"`
	Vision      bool `json:"vision,omitempty"`
	LongContext int  `json:"long_context,omitempty"`
	Speed       int  `json:"speed,omitempty"`
	Tools       bool `json:"tools,omitempty"`
	// Offline means nothing from this work may leave the computer.
	Offline bool `json:"offline,omitempty"`
}

// Candidate is one model that could do the work.
type Candidate struct {
	Name  string
	Bytes int64

	// Cloud models are only candidates when Enabled; Paid ones also need the
	// policy to allow paying, which nothing does by default.
	Cloud   bool
	Enabled bool
	Paid    bool

	// From measuring it here: seconds for one tool call, and whether the
	// call came back as a real call ("proper"), as prose ("text") or not at
	// all ("none"). Empty when it was never measured.
	Seconds  float64
	ToolCall string

	// Failures is how many recent calls failed or timed out.
	Failures int
}

// Policy is what the owner allowed and what the machine holds.
type Policy struct {
	AllowPaid bool
	// MemoryBytes is what a local model may occupy; 0 when unknown.
	MemoryBytes int64
	// Pins are the owner's own choices, by expert.
	Pins map[string]string
	// SpeedFloor is the least speed every pick asks for. 1 on a machine
	// without a graphics card, where every billion parameters is seconds of
	// waiting whatever the work is.
	SpeedFloor int
}

// Choice is the model to use, why, and what to try next.
type Choice struct {
	Model     string   `json:"model"`
	Reason    string   `json:"reason"`
	Fallbacks []string `json:"fallbacks,omitempty"`
}

// FailuresBeforeDemotion is how many failures move a model to the back.
const FailuresBeforeDemotion = 2

// Pick chooses the model for one expert.
func Pick(expert string, needs Needs, candidates []Candidate, policy Policy) (Choice, error) {
	type scored struct {
		c     Candidate
		score float64
		why   string
	}

	if needs.Speed < policy.SpeedFloor {
		needs.Speed = policy.SpeedFloor
	}

	var usable []scored

	var refused []string

	for _, c := range candidates {
		if why := unusable(c, needs, policy); why != "" {
			refused = append(refused, c.Name+": "+why)
			continue
		}

		score, why := fit(c, needs)
		usable = append(usable, scored{c, score, why})
	}

	if len(usable) == 0 {
		if len(refused) == 0 {
			return Choice{}, fmt.Errorf("no model is available for %s", expert)
		}

		return Choice{}, fmt.Errorf("no model fits %s (%s)", expert, strings.Join(refused, "; "))
	}

	sort.SliceStable(usable, func(i, j int) bool {
		di := usable[i].c.Failures >= FailuresBeforeDemotion
		dj := usable[j].c.Failures >= FailuresBeforeDemotion

		if di != dj {
			return !di
		}

		if usable[i].score != usable[j].score {
			return usable[i].score > usable[j].score
		}

		return usable[i].c.Name < usable[j].c.Name
	})

	order := make([]string, len(usable))
	for i, s := range usable {
		order[i] = s.c.Name
	}

	if pin := policy.Pins[expert]; pin != "" {
		for i, name := range order {
			if name == pin {
				rest := append(append([]string{}, order[:i]...), order[i+1:]...)

				return Choice{Model: pin, Reason: "pinned by you for " + expert, Fallbacks: rest}, nil
			}
		}

		top := usable[0]

		return Choice{
			Model:     top.c.Name,
			Reason:    "your pin " + pin + " is not available here; " + top.why,
			Fallbacks: order[1:],
		}, nil
	}

	top := usable[0]
	reason := top.why

	if top.c.Failures >= FailuresBeforeDemotion {
		reason += "; every other model is ruled out, although this one failed recently"
	}

	return Choice{Model: top.c.Name, Reason: reason, Fallbacks: order[1:]}, nil
}

// unusable says why a candidate may not be used at all, or "".
func unusable(c Candidate, needs Needs, policy Policy) string {
	switch {
	case c.Cloud && needs.Offline:
		return "the work must stay on this computer"
	case c.Cloud && !c.Enabled:
		return "its provider is not switched on"
	case c.Paid && !policy.AllowPaid:
		return "it costs money and paying is not allowed"
	case !c.Cloud && policy.MemoryBytes > 0 && c.Bytes > policy.MemoryBytes:
		return "it does not fit in this machine's memory"
	case needs.Vision && !TraitsOf(c.Name).Vision:
		return "it cannot see pictures"
	case needs.Tools && c.ToolCall == "none":
		return "it did not use a tool when measured"
	}

	return ""
}

// fit scores a usable candidate against the needs, with the reason in words.
func fit(c Candidate, needs Needs) (float64, string) {
	t := TraitsOf(c.Name)
	if t.Billions == 0 && c.Bytes > 0 {
		// Roughly two-thirds of a byte per parameter at the usual quantisation.
		t.Billions = float64(c.Bytes) / 0.6e9
	}

	var why []string

	score := 0.0

	// Bigger reasons better, with diminishing returns; asked-for depth
	// weighs it more.
	size := 0.0
	if t.Billions > 0 {
		size = min(t.Billions, 70) / 70
	}

	score += size * float64(1+needs.Reasoning)

	if needs.Coding > 0 && t.Coding {
		score += float64(needs.Coding)
		why = append(why, "made for code")
	}

	if needs.Vision {
		why = append(why, "sees pictures")
	}

	if needs.LongContext > 0 && t.LongContext {
		score += float64(needs.LongContext) / 2
		why = append(why, "reads long documents")
	}

	if needs.Tools {
		switch c.ToolCall {
		case "proper":
			score += 1
			why = append(why, "uses tools properly")
		case "text":
			score -= 0.5
		}
	}

	// Speed: a measured second costs more the more speed was asked for; an
	// unmeasured model is judged by its size.
	if needs.Speed > 0 {
		seconds := c.Seconds
		if seconds == 0 {
			seconds = 2 + t.Billions
		}

		score -= float64(needs.Speed) * seconds / 20

		if c.Seconds > 0 {
			why = append(why, fmt.Sprintf("answers a tool call in %.1f s here", c.Seconds))
		}
	}

	if c.Cloud {
		why = append(why, "cloud model you switched on")
	} else {
		why = append(why, "runs on this computer")
	}

	if t.Billions > 0 {
		why = append([]string{fmt.Sprintf("%.0fB parameters", t.Billions)}, why...)
	}

	return score, c.Name + ": " + strings.Join(why, ", ")
}

// Traits is what a model's name says about it.
type Traits struct {
	Billions    float64
	Coding      bool
	Vision      bool
	LongContext bool
}

var billions = regexp.MustCompile(`(?i)(?:^|[^a-z0-9.])(\d+(?:\.\d+)?)b(?:$|[^a-z])`)

/*
 * TraitsOf reads a model's traits from its name, the way Ollama names them:
 * "qwen2.5-coder:7b", "llava:13b", "llama3.2-vision:11b".
 *
 * Names are what is reliably there before anything has been measured. A
 * family this does not know is simply judged by its size.
 */
func TraitsOf(name string) Traits {
	n := strings.ToLower(name)

	var t Traits

	if m := billions.FindStringSubmatch(n); m != nil {
		t.Billions, _ = strconv.ParseFloat(m[1], 64)
	}

	for _, w := range []string{"coder", "code", "starcoder", "devstral", "codestral", "deepseek-coder"} {
		if strings.Contains(n, w) {
			t.Coding = true
		}
	}

	for _, w := range []string{"llava", "vision", "-vl", "moondream", "bakllava", "minicpm-v", "gemma3"} {
		if strings.Contains(n, w) {
			t.Vision = true
		}
	}

	for _, w := range []string{"qwen2.5", "qwen3", "llama3.1", "llama3.2", "llama3.3", "mistral-nemo", "gemma3", "phi4", "command-r"} {
		if strings.Contains(n, w) {
			t.LongContext = true
		}
	}

	return t
}
