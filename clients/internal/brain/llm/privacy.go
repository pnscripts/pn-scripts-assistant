package llm

import (
	"errors"
	"fmt"
)

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

	// ModeOpen allows a third-party model. The conversation goes to it, and so
	// does the memory recalled for that turn (see AllowsMemoryFor). The memory
	// database itself stays on this machine.
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

	return NotAllowed{Provider: provider, Mode: m}
}

/*
 * NotAllowed is a provider refused by privacy, as against one that could not
 * be reached.
 *
 * A type rather than a message, because the difference decides what a caller
 * should do about it and a caller cannot read a sentence. A company having an
 * outage is bad luck and worth working around; privacy refusing is a decision
 * somebody made, and working around it would be overruling them — quietly, in
 * the one part of this program where quiet is unacceptable.
 *
 * The wording is unchanged, since it is what somebody reads.
 */
type NotAllowed struct {
	Provider string
	Mode     Mode
}

func (n NotAllowed) Error() string {
	return fmt.Sprintf(
		"refusing to use the %q provider: privacy is set to %q, which keeps conversations "+
			"on this machine; set BRAIN_PRIVACY=open to allow it",
		n.Provider, n.Mode,
	)
}

// Forbidden reports whether an error is privacy refusing rather than something
// failing.
func Forbidden(err error) bool {
	var refused NotAllowed

	return errors.As(err, &refused)
}

// AllowsWeb reports whether the brain may reach the public internet.
func (m Mode) AllowsWeb() bool { return m != ModePrivate }

/*
 * AllowsMemoryFor reports whether what the brain has learned may be included
 * in a request to this provider.
 *
 * This used to be false for every third party in every mode, including open,
 * and the comment here said there was no configuration that changed it. The
 * reasoning was sound and is worth keeping written down: conversation is typed
 * deliberately, and memory is assembled from somebody's disk without them
 * composing it — so it is a dossier rather than a prompt, and it is the
 * richest part.
 *
 * It is no longer absolute because the person it was protecting said so. It
 * now follows the privacy mode: in open mode the memory recalled for a turn is
 * sent with it to the hosted provider, and Describe says so rather than
 * leaving it to be discovered. In every other mode it behaves exactly as it
 * always did. The memory database is never sent; only what a turn recalls.
 */
func AllowsMemoryFor(provider string, mode Mode) bool {
	return provider == Local || mode == ModeOpen
}

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
			Summary: "Conversations may be sent to a hosted model.",
			Detail: "In open mode, your messages and the memory recalled for that turn " +
				"are sent to the hosted provider you chose. The memory database itself " +
				"stays on your machine.",
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
