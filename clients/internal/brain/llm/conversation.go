package llm

import (
	"bytes"
	"encoding/json"
	"strings"
)

/*
 * The two rules every strict API agrees on, in one place.
 *
 * Both of them were learned the same way: something worked against Ollama and
 * failed against everybody else, and the failure did not look like what it
 * was. Arguments arrived as a string containing JSON and the error named a
 * tool. Tool output arrived answering nothing and the model behaved as though
 * it had never looked anything up. Neither is a provider being difficult —
 * they are the same conversation written down two different ways, and the
 * translating belongs here rather than in each client.
 */

// AsObject unwraps arguments a service sent as a string containing JSON.
//
// OpenAI and everything that copied its shape write tool arguments as
// {"arguments": "{\"path\":\"/etc/hosts\"}"} — a string. Ollama and Anthropic
// write an object. The loop hands whatever arrives to a tool that unmarshals
// it into a struct, so the string form failed with "cannot unmarshal string
// into Go value of type struct" and read like a tool that was broken.
//
// Anything that is not a quoted string is returned untouched, and a string
// that does not contain JSON is left alone too: turning "sorry" into sorry
// would trade a clear failure for an unparseable request body.
func AsObject(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)

	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		// Anthropic requires an object here even when a tool takes nothing.
		return json.RawMessage("{}")
	}

	if trimmed[0] != '"' {
		return trimmed
	}

	var inner string

	if err := json.Unmarshal(trimmed, &inner); err != nil {
		return trimmed
	}

	inner = strings.TrimSpace(inner)

	if inner == "" {
		return json.RawMessage("{}")
	}

	if !json.Valid([]byte(inner)) {
		return trimmed
	}

	return json.RawMessage(inner)
}

/*
 * Replayable rewrites a conversation so a strict API will accept it.
 *
 * A tool result is only a tool result when the message before it asked for
 * that exact call. The brain stores tool output in the transcript — so a later
 * turn still knows what was looked up — and reads it back with no id and
 * nothing in front of it. That is an orphan, and an orphan is a refused
 * request from every OpenAI-compatible service and a silent deletion by
 * Anthropic. It is the second turn of every conversation that ever used a
 * tool, which is why hosted models seemed to forget what they had just done.
 *
 * The orphan is not thrown away, because it is the evidence. It becomes an
 * ordinary message saying what a tool returned, which is what it actually is
 * by then: a fact from earlier, not an answer to a question still open.
 *
 * A call nobody answered is dropped rather than sent. Both APIs require every
 * call in a turn to be answered in the next one, so an unanswered call is a
 * refusal waiting to happen — and the assistant asking for something whose
 * outcome we do not know is worth less than the request going through.
 *
 * Ollama does not call this. It accepts the orphan and renders it sensibly,
 * and changing the one path that works is a risk with nothing to buy.
 */
func Replayable(messages []Message) []Message {
	// Which of an assistant's calls actually got an answer. A result only
	// counts when it follows the message that asked with nothing in between,
	// which is the shape both APIs describe.
	answered := make([]map[string]bool, len(messages))

	for i, m := range messages {
		if m.Role != RoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}

		got := make(map[string]bool)

		for j := i + 1; j < len(messages) && messages[j].Role == RoleTool; j++ {
			if id := messages[j].ToolCallID; id != "" {
				got[id] = true
			}
		}

		answered[i] = got
	}

	out := make([]Message, 0, len(messages))

	// open is what the assistant message just emitted is still owed. Anything
	// that is not a tool result closes it.
	var open map[string]bool

	for i, m := range messages {
		switch m.Role {
		case RoleAssistant:
			kept := m
			kept.ToolCalls = nil

			for _, c := range m.ToolCalls {
				if answered[i][c.ID] {
					kept.ToolCalls = append(kept.ToolCalls, c)
				}
			}

			open = nil

			if len(kept.ToolCalls) > 0 {
				open = make(map[string]bool, len(kept.ToolCalls))

				for _, c := range kept.ToolCalls {
					open[c.ID] = true
				}
			}

			// Anthropic refuses an assistant turn with nothing in it, and
			// there was never anything to say for one.
			if strings.TrimSpace(kept.Content) == "" && len(kept.ToolCalls) == 0 {
				continue
			}

			out = append(out, kept)

		case RoleTool:
			if m.ToolCallID != "" && open[m.ToolCallID] {
				out = append(out, m)
				continue
			}

			out = append(out, Message{Role: RoleUser, Content: whatAToolReturned(m)})

		default:
			open = nil

			out = append(out, m)
		}
	}

	// Anthropic refuses a final assistant message ending in whitespace, and a
	// streamed reply cut off mid-sentence often does.
	if last := len(out) - 1; last >= 0 && out[last].Role == RoleAssistant {
		out[last].Content = strings.TrimRight(out[last].Content, " \t\r\n")
	}

	return out
}

/*
 * withoutToolCalls turns a turn that used tools back into plain prose.
 *
 * For a call made with no tools offered — a spoken turn, or one the small-talk
 * model handles. Anthropic refuses a conversation that contains a tool_use
 * when nothing declares that tool, and the alternative, declaring the tools
 * again purely to make the history legal, would invite the model to call them
 * on exactly the turns that were meant not to.
 *
 * Stripping the calls leaves every result an orphan, which Replayable then
 * writes out as what it is. Nothing is lost but the shape.
 */
func withoutToolCalls(messages []Message) []Message {
	out := make([]Message, len(messages))

	for i, m := range messages {
		m.ToolCalls = nil
		out[i] = m
	}

	return out
}

// whatAToolReturned says plainly what an orphaned result is, so the evidence
// survives being moved out of the role that could no longer carry it.
func whatAToolReturned(m Message) string {
	if name := strings.TrimSpace(m.Name); name != "" {
		return "Earlier, the " + name + " tool returned:\n" + m.Content
	}

	return "Earlier, a tool returned:\n" + m.Content
}
