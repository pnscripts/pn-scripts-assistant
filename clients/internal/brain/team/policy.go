package team

import (
	"strings"

	"pn-scripts-assistant/internal/brain/llm"
)

/*
 * Which model an agent thinks with, which is not the same question as which
 * service it thinks through.
 *
 * These were one question and it was quietly wrong. The size roles — work,
 * quick, best — resolve to whatever is installed on this machine, which means
 * Ollama tags: "qwen3:8b", "llama3.2:3b". An agent pinned to Anthropic with
 * uses: best was therefore sending "qwen3:8b" to Anthropic as the model name.
 * Nothing checked, nothing complained, and it stayed invisible only because
 * nobody had pinned a hosted service to an agent yet.
 *
 * So a model name belongs to the service it names a model of. Locally, the
 * size roles are the right abstraction and stay exactly as they were. On a
 * hosted service, either the agent names a model that service actually has, or
 * nothing is sent and the service answers with its own default — which is the
 * one answer guaranteed to be a model that exists.
 */

/*
 * Needs is what a piece of work requires of a model, as against which model
 * somebody would prefer.
 *
 * Kept apart from the preference because they fail differently. A preference
 * that cannot be met is a small disappointment; a requirement that cannot be
 * met means the work cannot be done at all, and the honest answer is to say so
 * rather than to try it with a model that will produce something plausible.
 */
type Needs struct {
	// Tools is set when the work involves doing something rather than saying
	// something. A model that cannot call a tool will describe calling one.
	Tools bool

	// Vision is set when the work involves looking at a picture.
	Vision bool

	// Thinks is set when the work is hard enough to be worth a model that
	// reasons before answering, at the cost of the wait.
	Thinks bool

	/*
	 * Local keeps this agent on this machine whatever privacy allows.
	 *
	 * A narrowing, like everything else on an agent: privacy can forbid a
	 * hosted service and this cannot permit one. It is for the agent that
	 * handles the books or the post, where the answer to "may this leave"
	 * should not depend on which mode somebody left the program in.
	 */
	Local bool
}

// Wants reports whether anything is required at all.
func (n Needs) Wants() bool { return n.Tools || n.Vision || n.Thinks || n.Local }

/*
 * ModelOn is the model this agent should be sent to a particular service.
 *
 * Empty is a real answer and the right one for a hosted service with nothing
 * named: the provider then uses its own default, which is the only model name
 * this program can be certain exists there.
 */
func (a Agent) ModelOn(provider string, sizes llm.Sizes) string {
	// What somebody wrote by hand wins everywhere. An agent that names a model
	// is an agent whose owner has decided.
	for _, want := range a.Prefers {
		if want = strings.TrimSpace(want); want != "" {
			return want
		}
	}

	if llm.Elsewhere(provider) {
		return ""
	}

	return a.Model(sizes)
}

/*
 * Through is which service this agent should be asked through.
 *
 * Its own when it has one, otherwise the task's. An agent that must stay on
 * this machine is held there regardless, which is the one case where the
 * agent's setting outranks the task's — and it can only ever narrow, since the
 * local provider is the one privacy never forbids.
 */
func (a Agent) Through(taskProvider string) string {
	if a.Needs.Local {
		return llm.Local
	}

	if a.Provider != "" {
		return a.Provider
	}

	return taskProvider
}

/*
 * Needs written as words, because that is how the file reads.
 *
 * "needs: tools, local" rather than four boolean lines. A roster file is read
 * far more often than it is written, and a line somebody can take in at a
 * glance is worth the few lines of parsing here.
 */
func (n Needs) Written() string {
	var out []string

	for _, one := range []struct {
		word string
		on   bool
	}{
		{"tools", n.Tools},
		{"vision", n.Vision},
		{"thinks", n.Thinks},
		{"local", n.Local},
	} {
		if one.on {
			out = append(out, one.word)
		}
	}

	return strings.Join(out, ", ")
}

func readNeeds(value string) Needs {
	var n Needs

	for _, word := range splitList(strings.ToLower(value)) {
		switch word {
		case "tools":
			n.Tools = true
		case "vision":
			n.Vision = true
		case "thinks", "thinking", "reason":
			n.Thinks = true
		case "local", "here", "private":
			n.Local = true
		}
	}

	return n
}
