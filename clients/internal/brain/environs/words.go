package environs

import (
	"sort"
	"strings"
)

/*
 * The world, written out for a model to read.
 *
 * The hard part is not what to say, it is how little. A conversation turn on
 * this machine is already about six thousand tokens and takes ten minutes to
 * read on four cores; an inventory of eighty programs written out generously
 * would add a thousand more and buy nothing, because the model does not need
 * to be told that jq is version 1.7.1 in order to know it can use jq.
 *
 * So: one line per kind, names separated by commas, versions only where a
 * version changes what can be done with the thing, and a single line for what
 * is here but not ready. Everything the model needs in order to stop guessing,
 * and nothing it would have to read past.
 *
 * The order never changes. A block that reshuffles itself between turns is a
 * prompt prefix that cannot be reused, and on this hardware that costs
 * minutes — measured, not supposed.
 */

// HowMuchToSay bounds the block, in characters. Roughly 250 tokens, against
// a turn that is already six thousand.
const HowMuchToSay = 1000

// versionWorthSaying is the kinds where which version it is changes what can
// be done. A game engine's major version decides whether a project opens at
// all; jq's does not.
var versionWorthSaying = map[Kind]bool{
	Runtime: true,
	Engine:  true,
	Model:   true,
	Coder:   true,
}

// heading is what each kind is called in the block.
var heading = map[Kind]string{
	Model:       "Thinks with",
	Coder:       "Writes code with",
	Runtime:     "Languages",
	Engine:      "Game engines",
	Editor:      "Editors",
	Tool:        "Tools",
	Browser:     "Browsers",
	Integration: "Integrations",
	Service:     "Services",
	Ability:     "Can do",
}

/*
 * Words is the inventory, as the model is given it.
 *
 * most bounds the whole block; zero means HowMuchToSay. What is here comes
 * first and in full, because that is what changes an answer. What is missing
 * is summarised at the end and cut first when there is no room, because "you
 * do not have Unreal" is worth one clause and never worth a paragraph.
 */
func Words(things []Thing, most int) string {
	if most <= 0 {
		most = HowMuchToSay
	}

	byKind := map[Kind][]Thing{}
	var kinds []Kind

	for _, thing := range things {
		if thing.State == WantsInstalling || thing.State == Unknown {
			continue
		}

		if _, seen := byKind[thing.Kind]; !seen {
			kinds = append(kinds, thing.Kind)
		}

		byKind[thing.Kind] = append(byKind[thing.Kind], thing)
	}

	sort.Slice(kinds, func(i, j int) bool { return kindOrder(kinds[i]) < kindOrder(kinds[j]) })

	if len(kinds) == 0 && missingLine(things) == "" {
		// Nothing known yet, or nothing left after somebody's exclusions. A
		// heading with nothing under it is worse than silence: it tells the
		// model the machine is empty.
		return ""
	}

	var b strings.Builder

	b.WriteString("On this machine, now:\n")

	for _, kind := range kinds {
		line := strings.Join(named(byKind[kind], versionWorthSaying[kind]), ", ")
		if line == "" {
			continue
		}

		title := heading[kind]
		if title == "" {
			title = string(kind)
		}

		b.WriteString(title + ": " + line + "\n")
	}

	if missing := missingLine(things); missing != "" {
		b.WriteString(missing + "\n")
	}

	out := b.String()

	if len(out) <= most {
		return strings.TrimRight(out, "\n")
	}

	// Over budget: drop the missing line first, then trim whole lines from
	// the end, so what is cut is always the least useful thing left.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	for len(lines) > 1 && len(strings.Join(lines, "\n")) > most {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}

/*
 * named writes each thing, with the state said out loud when it is not
 * simply usable.
 *
 * "Claude Code (signed out)" rather than leaving it out: a model that is not
 * told a thing exists cannot suggest signing in to it, and a model told it
 * exists and is ready will try to use it and fail. Both failures come from
 * the same silence.
 */
func named(things []Thing, withVersion bool) []string {
	out := make([]string, 0, len(things))

	for _, thing := range things {
		name := thing.Title

		if withVersion && thing.Version != "" {
			name += " " + shortVersion(thing.Version)
		}

		switch thing.State {
		case Here:
		case WantsSignIn:
			name += " (not signed in)"
		case WantsSetting:
			name += " (needs setting up)"
		case WantsLicence:
			name += " (not licensed)"
		case Spent:
			name += " (nothing left today)"
		case Blocked:
			name += " (here, unusable)"
		default:
			continue
		}

		out = append(out, name)
	}

	return out
}

/*
 * missingLine is what is not here, in one clause.
 *
 * Only the things somebody would plausibly ask for — engines, languages,
 * coding agents — because "you do not have podman" is not worth a word until
 * somebody asks about podman, and the assistant can look again the moment
 * they do.
 */
func missingLine(things []Thing) string {
	var absent []string

	for _, thing := range things {
		if thing.State != WantsInstalling {
			continue
		}

		switch thing.Kind {
		case Engine, Runtime, Coder:
			absent = append(absent, thing.Title)
		}
	}

	if len(absent) == 0 {
		return ""
	}

	sort.Strings(absent)

	return "Not installed: " + strings.Join(absent, ", ") +
		" — you can say so, and offer to install what this program installs."
}

/*
 * shortVersion is the part of a version worth a model's attention.
 *
 * Godot calls itself 4.7.1.stable.official.a13da4feb, which is exactly right
 * in a bug report and thirty wasted characters in a prompt that is read on
 * every turn. Three numbers is what anybody decides anything by; the Thing
 * keeps the whole string for whatever compares versions properly.
 */
func shortVersion(version string) string {
	parts := strings.Split(version, ".")

	kept := make([]string, 0, 3)

	for _, part := range parts {
		if len(kept) == 3 || part == "" || !allDigits(part) {
			break
		}

		kept = append(kept, part)
	}

	if len(kept) == 0 {
		return version
	}

	return strings.Join(kept, ".")
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}
