package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/risk"
	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Integrations, in conversation.
 *
 * Three tools, and none of them can add a server: that is done in the
 * Integrations panel, by its owner, from the catalogue or by hand. A model
 * can say what there is, ask for one from the catalogue to be switched on —
 * which is always put to its owner with what it runs and where — and read
 * what a running one offers to be read.
 */

// ListIntegrations says what integrations there are and what each offers.
type ListIntegrations struct{ Gateway *Gateway }

func (ListIntegrations) Name() string { return "list_integrations" }

func (ListIntegrations) Description() string {
	return "List the integrations (MCP servers): which are approved and running, what each " +
		"offers, and which could be switched on from the catalogue."
}

func (ListIntegrations) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (ListIntegrations) Risk() tools.Risk                 { return tools.Safe }
func (ListIntegrations) Summarize(json.RawMessage) string { return "List the integrations" }

func (t ListIntegrations) Execute(context.Context, json.RawMessage) (string, error) {
	var b strings.Builder

	for _, v := range t.Gateway.List() {
		state := "in the catalogue, not added"

		switch {
		case v.Running:
			state = fmt.Sprintf("running (%s protocol %s), %d tools", v.Era, v.Protocol, len(v.Tools))
		case v.Approved:
			state = "approved, not running"
		case v.Listed:
			state = "added, not approved"
		}

		fmt.Fprintf(&b, "%s (%s): %s — %s\n", v.Title, v.ID, state, v.Why)

		for _, tool := range v.Tools {
			fmt.Fprintf(&b, "  %s\n", tool.As)
		}

		for _, r := range v.Resources {
			fmt.Fprintf(&b, "  resource %s — %s\n", r.URI, r.Name)
		}
	}

	return strings.TrimSpace(b.String()), nil
}

// ActivateIntegration switches on one integration from the catalogue.
type ActivateIntegration struct{ Gateway *Gateway }

func (ActivateIntegration) Name() string { return "activate_integration" }

func (ActivateIntegration) Description() string {
	return "Switch on an integration (MCP server) from the catalogue, for the owner's own " +
		"assistant. Always asks the owner first, saying what it runs and where."
}

func (ActivateIntegration) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {"integration": {"type": "string", "description": "Its id, from list_integrations."}},
		"required": ["integration"],
		"additionalProperties": false
	}`)
}

func (ActivateIntegration) Risk() tools.Risk { return tools.Mutating }

func integrationOf(raw json.RawMessage) string {
	var a struct {
		Integration string `json:"integration"`
	}

	json.Unmarshal(raw, &a)

	return a.Integration
}

// Consent: switching an integration on is always its owner's call.
func (ActivateIntegration) Consent(json.RawMessage) string { return "switching an integration on" }

func (ActivateIntegration) Weight(json.RawMessage) risk.Level { return risk.High }

func (t ActivateIntegration) Summarize(raw json.RawMessage) string {
	id := integrationOf(raw)

	s, ok := t.Gateway.Book.Get(id)
	if !ok {
		s, ok = FromCatalogue(id)
	}

	if !ok {
		return "Switch on the integration " + id + " (not in the catalogue — this will fail)"
	}

	line := fmt.Sprintf("Switch on %s: %s", s.Title, s.Where())

	if s.Version != "" {
		line += ", version " + s.Version
	}

	if s.Source != "" {
		line += ", from " + s.Source
	}

	if s.Licence != "" {
		line += ", licence " + s.Licence
	}

	return line + " — for your own assistant only"
}

func (t ActivateIntegration) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	id := integrationOf(raw)

	if _, ok := t.Gateway.Book.Get(id); !ok {
		if _, ok := FromCatalogue(id); !ok {
			return "", fmt.Errorf("there is no integration called %q; only the catalogue's can be switched on from here", id)
		}
	}

	if _, err := t.Gateway.Approve(id, nil); err != nil {
		return "", err
	}

	v, err := t.Gateway.Activate(ctx, id)
	if err != nil {
		return "", err
	}

	names := make([]string, 0, len(v.Tools))
	for _, tool := range v.Tools {
		names = append(names, tool.As)
	}

	return fmt.Sprintf("%s is running (%s protocol %s). Its tools: %s. Each asks before it changes anything.",
		v.Title, v.Era, v.Protocol, strings.Join(names, ", ")), nil
}

// ReadIntegration reads something a running integration offers.
type ReadIntegration struct{ Gateway *Gateway }

func (ReadIntegration) Name() string { return "read_integration" }

func (ReadIntegration) Description() string {
	return "Read one resource a running integration offers, by its address from list_integrations."
}

func (ReadIntegration) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"integration": {"type": "string"},
			"uri": {"type": "string", "description": "The resource's address."}
		},
		"required": ["integration", "uri"],
		"additionalProperties": false
	}`)
}

func (ReadIntegration) Risk() tools.Risk { return tools.Safe }

func (ReadIntegration) Summarize(raw json.RawMessage) string {
	var a struct{ Integration, URI string }

	json.Unmarshal(raw, &a)

	return "Read " + a.URI + " from " + a.Integration
}

func (t ReadIntegration) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Integration string `json:"integration"`
		URI         string `json:"uri"`
	}

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", err
	}

	return t.Gateway.ReadResource(ctx, a.Integration, a.URI, "")
}
