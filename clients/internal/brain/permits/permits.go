// Package permits decides what the brain is allowed to do on this machine.
//
// Privacy and permission are two different questions and this program had one
// answer for both. Privacy is about what leaves the machine — whether a model
// on somebody else's server may see what you said. Permission is about what
// happens on the machine in front of you: writing a file, running a command,
// sending a message, turning on a light. A brain set to keep everything local
// is not thereby allowed to delete things, and a brain allowed to use a
// hosted model is not thereby forbidden from writing a note.
//
// Tangling them meant the only way to let the brain do more was to let more
// leave, which is the wrong trade and nobody would choose it if it were
// written down as plainly as that.
//
// What was here before was two states: a tool either ran without asking or
// stopped every single time. There was no way to say "yes, and stop asking me
// about this one" — so somebody who uses a capability daily either approves it
// forever by hand or turns their brain into a thing that cannot act. That is
// the choice this package removes.
package permits

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/risk"
)

// FileName is where the standing grants live, beside the other small records
// the brain keeps about itself.
const FileName = "permissions.json"

// Answer is what to do about one attempt to act.
type Answer string

const (
	// Ask stops and puts it in front of a person. The default for anything
	// that changes something.
	Ask Answer = "ask"

	// Allow runs it and records that it ran.
	Allow Answer = "allow"

	// Refuse declines without asking. For something a person has decided the
	// brain should never do, so that saying no once is enough.
	Refuse Answer = "refuse"
)

/*
 * Freedom is how much the brain may do without being asked each time.
 *
 * Petar's words: "if a user decides to and tells it to do everything, then do
 * it based on the permissions". So there are three settings and the middle one
 * is the interesting one — it is what makes a granted permission mean
 * something rather than being a note nobody reads.
 */
type Freedom string

const (
	// AskEveryTime stops before anything that changes something. It is not
	// the default: a new brain starts on Everything (see config.Default), and
	// this is what somebody who wants to be asked sets it back to. It is also
	// how an unknown value is read, because that is the careful reading.
	AskEveryTime Freedom = "ask"

	// WhatIveAllowed runs what has been granted and asks about the rest. The
	// setting somebody arrives at after a week of using it.
	WhatIveAllowed Freedom = "granted"

	// Everything runs without asking. Still recorded, always — a permission to
	// act is not a permission to act unaccountably, and the record is the only
	// thing that makes this reversible.
	Everything Freedom = "everything"
)

// Known reports whether a freedom is one this program has; anything else is
// treated as the careful one.
func Known(f Freedom) bool {
	return f == AskEveryTime || f == WhatIveAllowed || f == Everything
}

// Grant is one standing decision about one capability.
type Grant struct {
	// Tool is the capability's name, as the registry knows it.
	Tool string `json:"tool"`

	/*
	 * Who this decision is about, or empty for everybody.
	 *
	 * Empty is the ordinary case and the one every permissions file written
	 * so far contains, which is why it has to mean everybody rather than
	 * nobody: a book written before there was an organisation keeps meaning
	 * what it meant.
	 *
	 * A decision about one agent never widens what anybody else may do. It
	 * either takes something away from that one, or gives it something its
	 * owner said yes to while watching it work.
	 */
	Who string `json:"who,omitempty"`

	Answer Answer `json:"answer"`

	// Given is when, so a person can see what they agreed to and when, which
	// is most of what makes a list of permissions reviewable rather than
	// frightening.
	Given time.Time `json:"given"`

	// Why is what was being done at the time, in the words they were shown.
	// A list of tool names is a list nobody can audit.
	Why string `json:"why,omitempty"`
}

/*
 * Book is the standing grants, plus the ones given only for this run.
 *
 * Two kinds and they are stored differently on purpose. "Always" belongs on
 * disk: it is a decision about how somebody wants to live with the program.
 * "Just this once, while I am doing this" belongs in memory and must not
 * outlive the program, or it is not what was agreed to.
 */
type Book struct {
	mu sync.RWMutex

	root string

	standing map[string]Grant
	session  map[string]bool
}

// Load reads the standing grants. A missing file is an empty book, not an
// error: a brain that has never been given a permission is the ordinary case.
func Load(root string) (*Book, error) {
	b := &Book{
		root:     root,
		standing: map[string]Grant{},
		session:  map[string]bool{},
	}

	raw, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		if os.IsNotExist(err) {
			return b, nil
		}

		return b, fmt.Errorf("reading %s: %w", FileName, err)
	}

	var list []Grant

	if err := json.Unmarshal(raw, &list); err != nil {
		/*
		 * A damaged file is an empty book, and loudly so.
		 *
		 * The alternative is refusing to start, and the thing that would be
		 * refused is somebody's assistant. Starting with no permissions is
		 * safe — everything falls back to asking — where starting with
		 * half-parsed permissions is not.
		 */
		return b, fmt.Errorf("%s is damaged, so nothing is granted until it is "+
			"fixed or deleted: %w", FileName, err)
	}

	for _, g := range list {
		if g.Tool == "" {
			continue
		}

		b.standing[key(g.Who, g.Tool)] = g
	}

	return b, nil
}

/*
 * Decide says what to do about one capability, given how much freedom the
 * brain has been given.
 *
 * Refuse always wins. Somebody who has said "never" has said it about the tool
 * and not about this moment, and a freedom setting that could override it
 * would make saying never pointless.
 */
func (b *Book) Decide(who, tool string, changesSomething bool, freedom Freedom) Answer {
	return b.DecideAt(who, tool, changesSomething, freedom, risk.Medium)
}

/*
 * DecideAt is Decide for a piece of work known to be this serious.
 *
 * One addition, and it only ever adds a question. A critical action is asked
 * about even where a standing grant or a yes-for-this-run would have let it
 * through, because those were given about a capability and a capability is
 * not a size: somebody who said "stop asking me about commands" said it about
 * listing folders and running tests, not about formatting a disk.
 *
 * Except on never stop, never refuse. Its owner was offered that as nothing
 * asking, including the risky ones, and chose it — so critical there is
 * recorded rather than asked, and the record says what it would have been.
 * Refusals still come first at every level: never is never.
 */
func (b *Book) DecideAt(who, tool string, changesSomething bool, freedom Freedom, level risk.Level) Answer {
	b.mu.RLock()
	defer b.mu.RUnlock()

	/*
	 * Refusals first, and the most restrictive wins.
	 *
	 * A refusal written about everybody stops this one too, and a refusal
	 * written about this one stops it even where everybody else may. That is
	 * the whole of "use the most restrictive applicable policy", and it is
	 * checked before anything else so that no later rule can talk its way
	 * past it — Freedom being set to everything included.
	 */
	if b.refuses(key("", tool)) || b.refuses(key(who, tool)) {
		return Refuse
	}

	// Reading and looking never needed permission and still do not: stopping
	// to approve every directory listing teaches somebody to click yes.
	if !changesSomething {
		return Allow
	}

	if freedom == Everything {
		return Allow
	}

	if level.AtLeast(risk.Critical) {
		return Ask
	}

	if b.session[key("", tool)] || b.session[key(who, tool)] {
		return Allow
	}

	if freedom == WhatIveAllowed {
		if b.allows(key("", tool)) || b.allows(key(who, tool)) {
			return Allow
		}
	}

	return Ask
}

func (b *Book) refuses(at string) bool {
	g, ok := b.standing[at]

	return ok && g.Answer == Refuse
}

func (b *Book) allows(at string) bool {
	g, ok := b.standing[at]

	return ok && g.Answer == Allow
}

/*
 * key is how a decision about one agent is told from one about everybody.
 *
 * A separator that cannot occur in either half, since both are validated
 * names: a tool name comes from the registry, and an agent's name is
 * lower-case letters, digits and underscores. Nothing has to guess where the
 * join is, and a book written before there was an organisation keys on the
 * bare tool name exactly as it always did.
 */
func key(who, tool string) string {
	if who == "" {
		return tool
	}

	return who + " \u00b7 " + tool
}

// Remember records a standing decision and writes it down.
func (b *Book) Remember(tool string, answer Answer, why string) error {
	return b.RememberFor("", tool, answer, why)
}

// RememberFor is the same decision, about one agent rather than everybody.
func (b *Book) RememberFor(who, tool string, answer Answer, why string) error {
	if tool == "" {
		return fmt.Errorf("which capability?")
	}

	if answer != Allow && answer != Refuse {
		return fmt.Errorf("a standing decision is allow or refuse, not %q", answer)
	}

	b.mu.Lock()

	b.standing[key(who, tool)] = Grant{
		Tool:   tool,
		Who:    who,
		Answer: answer,
		Given:  time.Now().UTC(),
		Why:    strings.TrimSpace(why),
	}

	b.mu.Unlock()

	return b.save()
}

// ForThisRun allows a capability until the program stops. Never written down.
func (b *Book) ForThisRun(tool string) { b.ForThisRunBy("", tool) }

// ForThisRunBy allows one agent a capability until the program stops.
func (b *Book) ForThisRunBy(who, tool string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.session[key(who, tool)] = true
}

// Forget removes a standing decision, so the capability goes back to asking.
func (b *Book) Forget(tool string) error { return b.ForgetFor("", tool) }

// ForgetFor removes one decision, so that capability goes back to asking.
func (b *Book) ForgetFor(who, tool string) error {
	b.mu.Lock()

	delete(b.standing, key(who, tool))
	delete(b.session, key(who, tool))

	b.mu.Unlock()

	return b.save()
}

// List is every standing decision, newest first, for showing somebody what
// they have agreed to.
func (b *Book) List() []Grant {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]Grant, 0, len(b.standing))

	for _, g := range b.standing {
		out = append(out, g)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Given.After(out[j].Given) })

	return out
}

// OnlyForThisRun is what has been allowed until the program stops, which is
// worth showing separately: it is the part that will be gone tomorrow.
func (b *Book) OnlyForThisRun() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]string, 0, len(b.session))

	for tool := range b.session {
		out = append(out, tool)
	}

	sort.Strings(out)

	return out
}

func (b *Book) save() error {
	b.mu.RLock()

	list := make([]Grant, 0, len(b.standing))

	for _, g := range b.standing {
		list = append(list, g)
	}

	b.mu.RUnlock()

	sort.Slice(list, func(i, j int) bool { return list[i].Tool < list[j].Tool })

	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	// Written whole and moved into place: a permissions file half written by a
	// program that was killed is a permissions file nobody can trust.
	temp := filepath.Join(b.root, FileName+".new")

	if err := os.WriteFile(temp, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", FileName, err)
	}

	return os.Rename(temp, filepath.Join(b.root, FileName))
}

/*
 * Means is what a freedom setting does, in a sentence somebody can act on.
 *
 * Beside the setting rather than in the interface, so that the words and the
 * behaviour cannot drift apart — a permissions screen whose description is out
 * of date with its code is worse than one with no description.
 */
func Means(f Freedom) string {
	/*
	 * What may happen on this machine, and nothing about what may leave it.
	 *
	 * These used to describe both, because the two were one switch. They are
	 * separate now (privacy has its own setting), and a sentence here that
	 * promised what leaves would be a promise this setting does not keep.
	 */
	switch f {
	case WhatIveAllowed:
		return "It does what you have already allowed and asks about the rest. " +
			"Every action is recorded."

	case Everything:
		return "It does anything it can without asking. Every action is recorded, " +
			"and files it overwrites are kept so they can be put back."

	default:
		return "It asks before anything that changes something. Every action is recorded."
	}
}
