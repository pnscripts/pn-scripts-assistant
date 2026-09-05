package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

/*
 * Saying what it can do, and how to ask for it.
 *
 * Everything here is reachable by saying a sentence, and none of it is
 * discoverable by saying a sentence. Somebody has to already know that "learn
 * everything", "answer only me" and "use the woman's voice" are things it
 * understands — and the way they were finding out was by asking the person who
 * wrote it, which is not a program that introduces itself.
 *
 * Asked what it can do, the model would otherwise answer from its own idea of
 * what an assistant is, which is a description of assistants in general. This
 * answers from the registry: every line is checked against a tool that is
 * actually loaded, so it cannot promise something that was never built or go
 * quietly out of date when something is added.
 */
type Introduce struct {
	// Loaded is which tools exist, so nothing is claimed that is not there.
	Loaded func() []string

	// Owner is who is being spoken to.
	Owner string
}

// canDo is one thing it can do, the tool that does it, and what to say.
type canDo struct {
	tool string
	what string
	say  string
}

/*
 * The introduction, in the order somebody meets it.
 *
 * Learning first, because that is what the program is for; then the settings,
 * because that is the question actually asked; then acting on the machine,
 * which is the part people do not expect a local assistant to do at all.
 */
var canDoList = []canDo{
	{"places_it_learns_from", "read a drive or folder and remember what is in it",
		"learn everything"},
	{"learn_from_folder", "read one folder or document now",
		"learn the documents in that folder"},
	{"what_you_know", "tell you what I have learned and where from",
		"what do you know about my projects"},
	{"decide_waiting", "keep or discard the things waiting for you",
		"approve everything waiting"},

	{"change_a_setting", "change how it behaves — speaking aloud, whose voice it " +
		"answers, turning the music down, which voice it uses",
		"answer only me"},
	{"set_wake_word", "change what I am called", "call yourself Cortex"},
	{"set_appearance", "change the colours", "make the core green"},
	{"what_am_i_hearing", "tell you what I heard and why I did or did not answer",
		"why did you ignore me"},
	{"how_fast_can_you_answer", "tell you how long answering takes here and what would help",
		"why are you so slow"},

	{"remind_me", "keep a reminder and say it when it is due",
		"remind me to call Anna at six"},
	{"read_file", "read a file and answer from it", "what is in that file"},
	{"write_file", "write or change a file", "write that down in notes.md"},
	{"run_command", "run a command", "run git status in that folder"},
	{"open_app", "open a program", "open the browser"},
	{"search_files", "find a file by name or content", "find the invoice from March"},
	{"look_at_screen", "look at what is on your screen", "what am I looking at"},
	{"put_it_back", "undo a change I made to a file", "put that file back"},
}

func (Introduce) Name() string { return "what_can_you_do" }

func (Introduce) Description() string {
	return "Introduce what this assistant can do and the words to say to ask for each " +
		"of them. Use this for \"what can you do\", \"how do I tell you to change " +
		"something\", \"what can I say\", \"how does this work\", or any question about " +
		"its own capabilities. Answer from this rather than from general knowledge: " +
		"it reports the tools actually loaded on this machine."
}

func (Introduce) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

// Safe: it describes itself and changes nothing.
func (Introduce) Risk() Risk { return Safe }

func (Introduce) Summarize(json.RawMessage) string { return "Say what it can do" }

func (t Introduce) Execute(context.Context, json.RawMessage) (string, error) {
	if t.Loaded == nil {
		return "", fmt.Errorf("nothing here can say what is loaded")
	}

	have := map[string]bool{}

	for _, name := range t.Loaded() {
		have[name] = true
	}

	var b strings.Builder

	owner := t.Owner
	if owner == "" {
		owner = "you"
	}

	b.WriteString("Say my name to start, and I stay in the conversation for a while " +
		"after, so you only need it once. Then:\n")

	var said int

	for _, c := range canDoList {
		if !have[c.tool] {
			continue
		}

		fmt.Fprintf(&b, "\n· %s — say %q", c.what, c.say)

		said++
	}

	if said == 0 {
		return "None of my tools are loaded, which means I can talk and nothing else.", nil
	}

	/*
	 * And the two things worth knowing before asking for any of it.
	 *
	 * Both are the kind of thing somebody finds out by being surprised, and
	 * being surprised by an assistant is how people stop trusting one.
	 */
	b.WriteString("\n\nAnything that changes something — a file, a setting, a whole " +
		"drive — stops and asks first, and says exactly what it is about to do. " +
		"Privacy is the one thing I will not change from a conversation: that is set " +
		"in the privacy panel, so that nothing said to me can talk me into sharing more.")

	return b.String(), nil
}
