package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Wrapped is one of a server's tools, as one of this program's.
 *
 * Everything about how it is treated is this program's decision, not the
 * server's. It changes something unless its owner has said it only reads — a
 * server calling its own tool read-only is a claim, shown as a hint and never
 * acted on. It is offered only to the agents its owner granted it to, on
 * projects that allow it, and never to anybody while privacy forbids what it
 * would do. Its description is the server's words, marked as such and cut
 * short. What it returns is handed on as information.
 */
type Wrapped struct {
	g      *Gateway
	server Server
	def    Tool
}

// MostSchema is the largest input schema offered to a model; anything bigger
// is a server spending somebody's context on itself.
const MostSchema = 8 << 10

// ToolName is what a server's tool is called here: mcp_<server>_<tool>,
// in the characters a tool name may have.
func ToolName(server, tool string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(tool) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if !strings.HasSuffix(b.String(), "_") {
				b.WriteRune('_')
			}
		}
	}

	name := "mcp_" + server + "_" + strings.Trim(b.String(), "_")

	if len(name) > 64 {
		name = name[:64]
	}

	return name
}

func (w *Wrapped) Name() string { return ToolName(w.server.ID, w.def.Name) }

func (w *Wrapped) Description() string {
	return "[" + w.server.Title + " integration — its own description, not this program's] " +
		oneLine(w.def.Description, 280)
}

func (w *Wrapped) Parameters() json.RawMessage {
	var shape struct {
		Type string `json:"type"`
	}

	if len(w.def.InputSchema) == 0 || len(w.def.InputSchema) > MostSchema ||
		json.Unmarshal(w.def.InputSchema, &shape) != nil || shape.Type != "object" {
		return json.RawMessage(`{"type":"object"}`)
	}

	return w.def.InputSchema
}

// Risk: changes something, unless its owner said it only reads.
func (w *Wrapped) Risk() tools.Risk {
	if listedIn(w.server.ReadOnly, w.def.Name) {
		return tools.Safe
	}

	return tools.Mutating
}

// Weight: leaving the machine, or a server saying it destroys, raises it.
func (w *Wrapped) Weight(json.RawMessage) risk.Level {
	level := risk.Medium

	if w.Risk() == tools.Safe {
		level = risk.Low
	}

	if w.server.Remote() {
		level = risk.Max(level, risk.High)
	}

	if a := w.def.Annotations; a != nil && a.DestructiveHint != nil && *a.DestructiveHint {
		level = risk.Max(level, risk.High)
	}

	return level
}

func (w *Wrapped) Summarize(args json.RawMessage) string {
	var compact bytes.Buffer

	json.Compact(&compact, args)

	shown := compact.String()
	if len(shown) > 300 {
		shown = shown[:300] + "…"
	}

	return fmt.Sprintf("%s — %s: %s %s", w.server.Title, w.server.Where(), w.def.Name, shown)
}

// GrantedTo: only the agents its owner named.
func (w *Wrapped) GrantedTo(agent string) bool { return listedIn(w.server.Agents, agent) }

// Server is which integration it belongs to.
func (w *Wrapped) Server() string { return w.server.ID }

// WritesNoFiles: what an integration does is governed by its grants, not by
// the project's folders — see tools.FileFree.
func (w *Wrapped) WritesNoFiles() {}

// Consent: what goes to a server elsewhere is sent in its owner's name, and
// is always asked, whatever has been allowed.
func (w *Wrapped) Consent(json.RawMessage) string {
	if w.LeavesTheMachine() {
		return "sending data to " + w.server.Title + ", off this machine"
	}

	return ""
}

// LeavesTheMachine is whether calling it sends anything elsewhere.
func (w *Wrapped) LeavesTheMachine() bool { return w.server.Remote() }

// AddedAtRunTime: it is replaced when its server is started again.
func (w *Wrapped) AddedAtRunTime() {}

// Cues: offered when the integration, or the tool itself, is mentioned — a
// server's tools crowding every turn would make the model choose worse at
// everything else.
func (w *Wrapped) Cues() []string {
	cues := []string{"integration", "mcp", strings.ToLower(w.server.ID), strings.ToLower(w.def.Name), w.Name()}

	for _, word := range strings.Fields(strings.ToLower(w.server.Title)) {
		if len(word) > 3 {
			cues = append(cues, word)
		}
	}

	return cues
}

// MostResult is as much of an answer as is handed on.
const MostResult = 12000

func (w *Wrapped) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	out, _, err := w.ExecuteShowing(ctx, args)

	return out, err
}

func (w *Wrapped) ExecuteShowing(ctx context.Context, args json.RawMessage) (string, []store.Evidence, error) {
	l, err := w.g.client(w.server.ID)
	if err != nil {
		return "", nil, err
	}

	result, err := l.client.CallTool(ctx, w.def.Name, args)

	text := ""
	if err == nil {
		text = describe(result)
	}

	w.g.note(w.server.ID, "call", w.def.Name, fmt.Sprintf("arguments %s; answer %s", digestOf(string(args), nil), digestOf(text, err)))

	evidence := []store.Evidence{{Kind: store.EvidenceCommand, Subject: "integration " + w.server.ID + ": " + w.def.Name,
		Detail: digestOf(text, err), OK: err == nil && !result.IsError}}

	if err != nil {
		return "", evidence, fmt.Errorf("%s", w.g.scrub(w.server.ID, err.Error()))
	}

	text = w.g.scrub(w.server.ID, text)

	if len(text) > MostResult {
		text = text[:MostResult] + "\n[…cut short]"
	}

	if result.IsError {
		text = "The integration reported an error:\n" + text
	}

	return untrusted(w.server.Title, text), evidence, nil
}

// describe is an answer as text: words kept, pictures and files described.
func describe(r CallResult) string {
	var b strings.Builder

	for _, c := range r.Content {
		switch c.Type {
		case "text":
			b.WriteString(c.Text + "\n")
		case "image", "audio":
			fmt.Fprintf(&b, "[%s: %s, %d bytes, not shown]\n", c.Type, c.MimeType, len(c.Data)*3/4)
		case "resource":
			if c.Resource != nil {
				if c.Resource.Text != "" {
					b.WriteString(c.Resource.Text + "\n")
				} else {
					fmt.Fprintf(&b, "[%s: binary, not shown]\n", c.Resource.URI)
				}
			}
		case "resource_link":
			fmt.Fprintf(&b, "[a link to %s]\n", c.URI)
		}
	}

	if b.Len() == 0 && len(r.StructuredContent) > 0 {
		b.Write(r.StructuredContent)
	}

	return strings.TrimSpace(b.String())
}
