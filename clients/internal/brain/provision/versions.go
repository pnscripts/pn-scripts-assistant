package provision

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

/*
 * Which versions will do.
 *
 * A package says "Godot 4.2 or later, before 5", and the answer has to be the
 * same whoever reads it. So the rule is small and written out: comparisons
 * joined by spaces or commas, every one of which must hold. A bare version
 * means that version and anything under it — "4.7" is 4.7.0 and 4.7.2 alike.
 *
 * Versions are compared as numbers, part by part, because 4.10 comes after
 * 4.9 and sorts before it as text. Whatever follows the numbers — "stable",
 * "f1", a build hash — is ignored: it says which build, not which version.
 */

// Rule is every comparison a version must pass.
type Rule struct {
	written string
	parts   []comparison
}

type comparison struct {
	op      string
	version []int
	prefix  bool
}

var ruleWord = regexp.MustCompile(`^(>=|<=|==|=|>|<)?\s*v?(\d+(?:\.\d+)*)(\.x|\.\*)?$`)

// ParseRule reads a rule. Empty is no rule at all.
func ParseRule(written string) (Rule, error) {
	r := Rule{written: strings.TrimSpace(written)}

	for _, word := range strings.FieldsFunc(r.written, func(c rune) bool { return c == ',' || c == ' ' }) {
		m := ruleWord.FindStringSubmatch(word)
		if m == nil {
			return Rule{}, fmt.Errorf("%q is not a version rule this reads — use forms like >=4.2 <5", word)
		}

		c := comparison{op: m[1], version: numbers(m[2])}

		if c.op == "" || c.op == "==" {
			c.op = "="
			c.prefix = true
		}

		r.parts = append(r.parts, c)
	}

	return r, nil
}

// Empty is a rule that allows anything.
func (r Rule) Empty() bool { return len(r.parts) == 0 }

func (r Rule) String() string { return r.written }

// Allows reports whether a version passes every comparison.
func (r Rule) Allows(version string) bool {
	have := numbers(version)

	if len(have) == 0 {
		return r.Empty()
	}

	for _, c := range r.parts {
		order := compare(have, c.version)

		switch c.op {
		case ">=":
			if order < 0 {
				return false
			}
		case ">":
			if order <= 0 {
				return false
			}
		case "<=":
			if order > 0 {
				return false
			}
		case "<":
			if order >= 0 {
				return false
			}
		case "=":
			if !startsWith(have, c.version) {
				return false
			}
		}
	}

	return true
}

// Newer reports whether b is a later version than a. Unreadable is never
// newer: telling somebody an update exists when it does not is worse than
// saying nothing.
func Newer(a, b string) bool {
	x, y := numbers(a), numbers(b)

	if len(x) == 0 || len(y) == 0 {
		return false
	}

	return compare(y, x) > 0
}

var leading = regexp.MustCompile(`\d+(?:\.\d+)*`)

// numbers is the version's numeric parts: "v4.7.2-stable" is 4, 7, 2, and
// "6000.0.23f1" is 6000, 0, 23.
func numbers(version string) []int {
	m := leading.FindString(strings.TrimPrefix(strings.TrimSpace(version), "v"))
	if m == "" {
		return nil
	}

	var out []int

	for _, part := range strings.Split(m, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			break
		}

		out = append(out, n)
	}

	return out
}

// compare is -1, 0 or 1, with missing parts read as zero: 4.7 is 4.7.0.
func compare(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int

		if i < len(a) {
			x = a[i]
		}

		if i < len(b) {
			y = b[i]
		}

		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}

	return 0
}

func startsWith(have, want []int) bool {
	if len(want) > len(have) {
		return compare(have, want) == 0
	}

	for i := range want {
		if have[i] != want[i] {
			return false
		}
	}

	return true
}
