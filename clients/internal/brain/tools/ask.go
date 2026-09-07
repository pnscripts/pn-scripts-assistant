package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

/*
 * Asking, instead of guessing or ploughing on.
 *
 * Two of Petar's complaints are the same missing thing. "Understand and ask if
 * not for prompt" — when a request has two readings, pick neither and ask.
 * "Understand and ask when thinking if there is an issue" — when the work hits
 * something that changes what was asked for, stop and say so.
 *
 * The brain had no way to do either. Every turn had to end in an answer or an
 * approval, so a model that was unsure could only guess confidently, and a
 * model that hit a problem could only carry on around it or fail quietly. Both
 * produce the same thing from a chair: an assistant that seems certain and is
 * wrong.
 *
 * A tool rather than a convention, because a convention in the prompt is
 * something a model does when it remembers to. A tool is in the list it reads
 * every turn, it is named, and the turn ends when it is called — see the loop,
 * which stops there rather than feeding the question back to the model. That
 * last part matters: a model asked to ask a question will otherwise answer it
 * itself on the next step.
 */

// Ask stops and puts a question to the owner.
type Ask struct {
	// Owner is who is being asked, so the question is addressed rather than
	// broadcast. Empty is fine.
	Owner string
}

// Name is short because the model types it often, and plain because the model
// has to recognise it as the thing to do when it does not know.
func (Ask) Name() string { return "ask_first" }

func (Ask) Description() string {
	return "Stop and ask the owner a question instead of guessing. " +
		"Use when the request has more than one reasonable reading and the " +
		"readings lead to different work; when something needed is missing; " +
		"or when what you found while working changes what was asked for. " +
		"Prefer this over picking an interpretation and hoping. It ends your " +
		"turn — the answer comes back as their next message."
}

func (Ask) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {
				"type": "string",
				"description": "The question, in one or two sentences. Ask the real question, not 'shall I continue'."
			},
			"why": {
				"type": "string",
				"description": "Optional: what you found that made you ask, in one line."
			},
			"options": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Optional: the readings you are choosing between, if there are two or three obvious ones."
			}
		},
		"required": ["question"],
		"additionalProperties": false
	}`)
}

// Safe: it changes nothing. Asking must never itself need permission, or the
// brain would need approval to admit it is unsure.
func (Ask) Risk() Risk { return Safe }

func (Ask) Summarize(args json.RawMessage) string {
	var a struct {
		Question string `json:"question"`
	}

	json.Unmarshal(args, &a)

	if a.Question == "" {
		return "Ask a question"
	}

	return "Ask: " + a.Question
}

/*
 * Execute never runs.
 *
 * The loop recognises this tool before executing anything and ends the turn
 * with the question. This exists so the tool satisfies the interface and so
 * that, if the loop ever stops special-casing it, the failure is a sentence
 * saying what went wrong rather than a silently swallowed question.
 */
func (Ask) Execute(context.Context, json.RawMessage) (string, error) {
	return "", fmt.Errorf("a question should have ended the turn and did not")
}

// Question is what was asked, pulled out of the call for the loop to end on.
type Question struct {
	Question string   `json:"question"`
	Why      string   `json:"why"`
	Options  []string `json:"options"`
}

// ReadQuestion parses the arguments of an ask_first call.
func ReadQuestion(args json.RawMessage) (Question, bool) {
	var q Question

	if err := json.Unmarshal(args, &q); err != nil {
		return Question{}, false
	}

	q.Question = strings.TrimSpace(q.Question)

	if q.Question == "" {
		return Question{}, false
	}

	return q, true
}

/*
 * Text is the question as it should appear in the conversation.
 *
 * Built here rather than in the loop so the shape is testable: the reason
 * first when there is one, because "I found X, so which did you mean" is a
 * question somebody can answer and "which did you mean" on its own is one they
 * have to reconstruct the context for.
 */
func (q Question) Text() string {
	var b strings.Builder

	if q.Why != "" {
		b.WriteString(strings.TrimSpace(q.Why))

		if !strings.HasSuffix(b.String(), ".") && !strings.HasSuffix(b.String(), "?") {
			b.WriteString(".")
		}

		b.WriteString(" ")
	}

	b.WriteString(q.Question)

	// The options as a short list, because two readings written out are much
	// easier to choose between than the same two described in a sentence.
	kept := make([]string, 0, len(q.Options))

	for _, o := range q.Options {
		if o = strings.TrimSpace(o); o != "" {
			kept = append(kept, o)
		}
	}

	if len(kept) > 0 {
		b.WriteString("\n")

		for _, o := range kept {
			b.WriteString("\n  · ")
			b.WriteString(o)
		}
	}

	return b.String()
}
