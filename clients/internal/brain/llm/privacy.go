package llm

import "fmt"

// Mode is what is allowed to leave this machine.
//
// This exists because the brain accumulates an unusually complete picture of
// one person — every project they own and where it lives, the documents on
// their disk, what they were doing and when. Sent to a third party, that is not
// "a prompt", it is a dossier.
//
// The rules are enforced in code rather than described in the system prompt,
// because a prompt is a request to a model that can be argued with — by the
// user, by a web page it reads, or by its own confusion. This cannot.
type Mode string

const (
	// ModePrivate is the only setting under which "no personal information is
	// shared" is a guarantee rather than an intention. Local model, local
	// embeddings, no web.
	ModePrivate Mode = "private"

	// ModeResearch keeps the model local, so memory and conversation stay put,
	// but the brain may search and read the web. Search queries do leave, and
	// the model composes them, so this is a real if narrow disclosure.
	ModeResearch Mode = "research"

	// ModeOpen allows a third-party model. Conversation goes to it; recalled
	// memory does not — that is withheld at every level, because it is the part
	// the user never chose to type.
	ModeOpen Mode = "open"
)

// ParseMode reads a configured value.
//
// An unrecognised value means a typo in configuration, and the safe reading of
// a typo is the strict one.
func ParseMode(s string) Mode {
	switch Mode(lower(s)) {
	case ModePrivate, ModeResearch, ModeOpen:
		return Mode(lower(s))
	default:
		return ModePrivate
	}
}

// Local is the name of the only provider that runs on this machine. Anything
// else is, by definition, somebody else's computer.
const Local = "ollama"

// AllowsProvider reports whether this provider may be used at all.
func (m Mode) AllowsProvider(provider string) bool {
	return provider == Local || m == ModeOpen
}

// GuardProvider is the enforcement point. Every path that selects a provider
// goes through it, so no caller can opt out by forgetting to check.
func (m Mode) GuardProvider(provider string) error {
	if m.AllowsProvider(provider) {
		return nil
	}

	return fmt.Errorf(
		"refusing to use the %q provider: privacy is set to %q, which keeps conversations "+
			"on this machine; set BRAIN_PRIVACY=open to allow it",
		provider, m,
	)
}

// AllowsWeb reports whether the brain may reach the public internet.
func (m Mode) AllowsWeb() bool { return m != ModePrivate }

// AllowsMemoryFor reports whether what the brain has learned may be included in
// a request to this provider.
//
// False for every third party, in every mode — including open. Conversation is
// typed deliberately; memory is assembled from a person's disk without them
// composing it, so it is not ours to forward, and it is the richest part.
//
// This deliberately does not consult the mode. There is no configuration that
// turns it on, because there is no setting at which forwarding somebody's
// assembled dossier is the behaviour they expected.
func AllowsMemoryFor(provider string) bool { return provider == Local }

// Description is how a setting is explained in the interface.
type Description struct {
	Mode    Mode   `json:"mode"`
	Summary string `json:"summary"`
	Detail  string `json:"detail"`
}

// Describe says what the setting actually means, in plain words.
func (m Mode) Describe() Description {
	switch m {
	case ModeResearch:
		return Description{
			Mode:    ModeResearch,
			Summary: "Your data stays here; the brain may read the web.",
			Detail: "Conversations and memory stay on this machine. Search queries the " +
				"brain composes do reach the search engine.",
		}
	case ModeOpen:
		return Description{
			Mode:    ModeOpen,
			Summary: "Conversations may be sent to a third-party model.",
			Detail: "What you type can go to Anthropic. What the brain has learned about " +
				"you is still withheld.",
		}
	default:
		return Description{
			Mode:    ModePrivate,
			Summary: "Nothing leaves this machine.",
			Detail: "Local model and local embeddings only. Web search and page fetching " +
				"are switched off.",
		}
	}
}

func lower(s string) string {
	out := []rune(s)

	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + ('a' - 'A')
		}
	}

	return string(out)
}
