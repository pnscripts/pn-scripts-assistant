package proof

import (
	"regexp"
	"strings"
)

/*
 * Comparing two versions of a text, word by word.
 *
 * "What did you change?" is the first question after any rewrite, and a
 * model asked it describes its changes from memory — which is to say it
 * guesses. The answer is computed here instead: the longest run of words
 * both versions share, and everything else marked as taken out or put in.
 *
 * Lines first, then words within the lines that differ, so a long document
 * with three edits costs three small comparisons rather than one enormous
 * one; a block too large to compare word by word is shown as replaced whole.
 */

// Part is one run of a comparison: the same in both, removed or added.
type Part struct {
	Kind string `json:"kind"` // same, removed, added
	Text string `json:"text"`
}

// Diff is two versions compared.
type Diff struct {
	Parts   []Part `json:"parts"`
	Removed int    `json:"words_removed"`
	Added   int    `json:"words_added"`
	Same    int    `json:"words_same"`
}

// Changed reports whether the versions differ at all.
func (d Diff) Changed() bool { return d.Removed+d.Added > 0 }

// Marked is the comparison as text: [-taken out-] and {+put in+}.
func (d Diff) Marked() string {
	var b strings.Builder

	for _, p := range d.Parts {
		switch p.Kind {
		case "removed":
			b.WriteString("[-" + p.Text + "-]")
		case "added":
			b.WriteString("{+" + p.Text + "+}")
		default:
			b.WriteString(p.Text)
		}
	}

	return b.String()
}

// tokenPattern splits into words, runs of space, and single other marks, so
// a changed comma is a changed comma and not a changed word.
var tokenPattern = regexp.MustCompile(`\s+|[\p{L}\p{M}\p{N}'’_-]+|.`)

// most is the largest word-by-word comparison made in one piece: a table of
// this many cells is a few megabytes.
const most = 2_000_000

// Compare compares before with after.
func Compare(before, after string) Diff {
	var d Diff

	a, b := splitLines(before), splitLines(after)

	for _, op := range lcs(a, b) {
		switch op.kind {
		case "same":
			d.add("same", op.a)
		case "replace":
			d.words(op.a, op.b)
		}
	}

	return d
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}

	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

func (d *Diff) words(before, after string) {
	a, b := tokenPattern.FindAllString(before, -1), tokenPattern.FindAllString(after, -1)

	if len(a)*len(b) > most {
		d.add("removed", before)
		d.add("added", after)

		return
	}

	for _, op := range lcs(a, b) {
		switch op.kind {
		case "same":
			d.add("same", op.a)
		case "replace":
			d.add("removed", op.a)
			d.add("added", op.b)
		}
	}
}

// add appends text to the comparison, joining it to a run of the same kind
// and counting its words.
func (d *Diff) add(kind, text string) {
	if text == "" {
		return
	}

	n := len(counted.FindAllString(text, -1))

	switch kind {
	case "removed":
		d.Removed += n
	case "added":
		d.Added += n
	default:
		d.Same += n
	}

	// A change of punctuation or spacing alone still counts as a change.
	if n == 0 && kind != "same" {
		if kind == "removed" {
			d.Removed++
		} else {
			d.Added++
		}
	}

	if last := len(d.Parts) - 1; last >= 0 && d.Parts[last].Kind == kind {
		d.Parts[last].Text += text

		return
	}

	d.Parts = append(d.Parts, Part{Kind: kind, Text: text})
}

var counted = regexp.MustCompile(`[\p{L}\p{N}]+`)

type op struct {
	kind string // same or replace
	a, b string
}

/*
 * lcs compares two lists of pieces and returns runs: pieces both share, and
 * between them what one had in place of the other. Common ends are taken
 * off first, which is most of any real edit.
 */
func lcs(a, b []string) []op {
	var head, tail []op

	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		head = append(head, op{kind: "same", a: a[0]})
		a, b = a[1:], b[1:]
	}

	for len(a) > 0 && len(b) > 0 && a[len(a)-1] == b[len(b)-1] {
		tail = append([]op{{kind: "same", a: a[len(a)-1]}}, tail...)
		a, b = a[:len(a)-1], b[:len(b)-1]
	}

	var middle []op

	switch {
	case len(a) == 0 && len(b) == 0:
	case len(a) == 0 || len(b) == 0 || len(a)*len(b) > most:
		middle = []op{{kind: "replace", a: strings.Join(a, ""), b: strings.Join(b, "")}}
	default:
		middle = table(a, b)
	}

	return append(append(head, middle...), tail...)
}

func table(a, b []string) []op {
	n, m := len(a), len(b)

	// lengths[i][j] is the longest common run of a[i:] and b[j:].
	lengths := make([][]int32, n+1)
	for i := range lengths {
		lengths[i] = make([]int32, m+1)
	}

	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lengths[i][j] = lengths[i+1][j+1] + 1
			} else if lengths[i+1][j] >= lengths[i][j+1] {
				lengths[i][j] = lengths[i+1][j]
			} else {
				lengths[i][j] = lengths[i][j+1]
			}
		}
	}

	var (
		out          []op
		gone, coming strings.Builder
	)

	flush := func() {
		if gone.Len()+coming.Len() > 0 {
			out = append(out, op{kind: "replace", a: gone.String(), b: coming.String()})
			gone.Reset()
			coming.Reset()
		}
	}

	i, j := 0, 0

	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			flush()
			out = append(out, op{kind: "same", a: a[i]})
			i++
			j++
		case lengths[i+1][j] >= lengths[i][j+1]:
			gone.WriteString(a[i])
			i++
		default:
			coming.WriteString(b[j])
			j++
		}
	}

	for ; i < n; i++ {
		gone.WriteString(a[i])
	}

	for ; j < m; j++ {
		coming.WriteString(b[j])
	}

	flush()

	return out
}
