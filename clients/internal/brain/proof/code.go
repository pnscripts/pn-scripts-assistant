package proof

import "strings"

/*
 * Code is not prose, so the proofreader leaves it alone.
 *
 * Checked against the 137 skills of the built-in pack, nine findings in ten
 * were inside Markdown code: `!important` read as a missing space, SQL's
 * brackets as unbalanced, "quotes" in a shell line as Bulgarian slips. A
 * proofreader that marks every line of a code sample teaches somebody to stop
 * reading its findings, the right ones included.
 *
 * Two kinds of code are recognised, the two every Markdown reader agrees on:
 * fenced blocks (``` or ~~~ at the start of a line, closed by the same
 * character at least as many times, or running to the end) and inline code
 * (a run of backticks closed by a run of the same length within the same
 * paragraph). Indented blocks are not: four spaces at the start of a line are
 * just as often a letter's indentation, and skipping a paragraph of somebody's
 * prose would be the worse mistake.
 */

// span is a byte range [at, end) of text.
type span struct{ at, end int }

// codeSpans lists where the code is in text, in order.
func codeSpans(text string) []span {
	var out []span

	// Fenced blocks first; inline code is looked for only between them.
	fences := fencedBlocks(text)

	from := 0

	for _, f := range fences {
		out = append(out, inlineCode(text, from, f.at)...)
		out = append(out, f)
		from = f.end
	}

	return append(out, inlineCode(text, from, len(text))...)
}

// fencedBlocks finds ``` and ~~~ blocks, each from its opening line to the end
// of its closing line.
func fencedBlocks(text string) []span {
	var out []span

	open := -1

	var fence byte

	length := 0

	for at := 0; at < len(text); {
		end := strings.IndexByte(text[at:], '\n')
		if end < 0 {
			end = len(text)
		} else {
			end += at + 1
		}

		line := text[at:end]
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)

		if indent <= 3 && len(trimmed) >= 3 && (trimmed[0] == '`' || trimmed[0] == '~') {
			c := trimmed[0]
			n := 0

			for n < len(trimmed) && trimmed[n] == c {
				n++
			}

			switch {
			case open < 0 && n >= 3:
				// An opening ``` may not have a backtick in its info string.
				if c != '`' || !strings.Contains(trimmed[n:], "`") {
					open, fence, length = at, c, n
				}
			case open >= 0 && c == fence && n >= length && strings.TrimSpace(trimmed[n:]) == "":
				out = append(out, span{open, end})
				open = -1
			}
		}

		at = end
	}

	if open >= 0 {
		out = append(out, span{open, len(text)})
	}

	return out
}

// inlineCode finds `code` between from and to: a run of backticks closed by a
// run of exactly the same length before the paragraph ends. An unclosed run is
// literal backticks, as Markdown reads it.
func inlineCode(text string, from, to int) []span {
	var out []span

	for i := from; i < to; {
		if text[i] != '`' {
			i++

			continue
		}

		n := run(text, i, to)

		closing := -1

		for k := i + n; k < to; {
			if text[k] == '\n' && blankLineAt(text, k+1, to) {
				break
			}

			if text[k] != '`' {
				k++

				continue
			}

			m := run(text, k, to)
			if m == n {
				closing = k

				break
			}

			k += m
		}

		if closing < 0 {
			i += n

			continue
		}

		out = append(out, span{i, closing + n})
		i = closing + n
	}

	return out
}

// run is how many backticks start at i.
func run(text string, i, to int) int {
	n := 0
	for i+n < to && text[i+n] == '`' {
		n++
	}

	return n
}

// blankLineAt reports whether the line starting at i holds nothing but spaces.
func blankLineAt(text string, i, to int) bool {
	for ; i < to; i++ {
		switch text[i] {
		case ' ', '\t', '\r':
		case '\n':
			return true
		default:
			return false
		}
	}

	return true
}

/*
 * withoutCode is text with every code span blanked to spaces, newlines kept,
 * so every byte offset found in it is an offset in text.
 */
func withoutCode(text string, code []span) string {
	if len(code) == 0 {
		return text
	}

	b := []byte(text)

	for _, s := range code {
		for i := s.at; i < s.end; i++ {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
	}

	return string(b)
}

// outsideCode keeps the findings that touch no code at all. A finding that
// reaches into a blanked span was found in blanks, not in what was written.
func outsideCode(in []Finding, code []span) []Finding {
	if len(code) == 0 {
		return in
	}

	out := in[:0]

	for _, f := range in {
		end := f.At + f.Length
		if f.Length == 0 {
			end = f.At + 1
		}

		touches := false

		for _, s := range code {
			if f.At < s.end && end > s.at {
				touches = true

				break
			}
		}

		if !touches {
			out = append(out, f)
		}
	}

	return out
}
