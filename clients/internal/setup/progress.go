package setup

import (
	"regexp"
	"strconv"
	"strings"
)

/*
 * How far into the current piece, read off the end of its own output.
 *
 * Every installer here reports progress and no two report it the same way: a
 * download counts bytes, ollama draws a percentage in a bar it redraws with
 * escape codes, and cmake prefixes each line with one in brackets. Rather than
 * teach each installer to report progress in a common shape — which would mean
 * changing four things that work — the last line that carries a number is read
 * here.
 *
 * The end, not the whole: the log holds every earlier step too, and a
 * percentage found near the top would be the last one from a piece that
 * finished minutes ago. Returns -1 when nothing recent says, which the page
 * draws as a bar with no position rather than as zero — "starting" and "no
 * idea" are different things and a bar stuck at 0% says the wrong one.
 */

var (
	// "[ 93%]" from cmake, and ollama's "  72%" inside its redrawn bar.
	percentPattern = regexp.MustCompile(`(\d{1,3})%`)

	// "3.4 GB of 4.7 GB" and "112MB of 1.3GB" from the downloader.
	ofPattern = regexp.MustCompile(`([\d.]+)\s*([KMG]i?B)\s+of\s+([\d.]+)\s*([KMG]i?B)`)
)

func percentOf(log string) int {
	if strings.TrimSpace(log) == "" {
		return -1
	}

	// The tail only. Escape codes are stripped because ollama redraws its bar
	// in place, so the "line" holding the current number is full of them.
	tail := log
	if len(tail) > 4000 {
		tail = tail[len(tail)-4000:]
	}

	lines := strings.Split(strings.ReplaceAll(tail, "\r", "\n"), "\n")

	for i := len(lines) - 1; i >= 0; i-- {
		line := stripEscapes(lines[i])

		if m := ofPattern.FindStringSubmatch(line); m != nil {
			done := bytesOf(m[1], m[2])
			total := bytesOf(m[3], m[4])

			if total > 0 {
				return clampPercent(int(done * 100 / total))
			}
		}

		if m := percentPattern.FindStringSubmatch(line); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				return clampPercent(n)
			}
		}
	}

	return -1
}

func clampPercent(n int) int {
	if n < 0 {
		return 0
	}

	if n > 100 {
		return 100
	}

	return n
}

/*
 * bytesOf turns "3.4" and "GB" into a number to compare with another.
 *
 * The unit was normalised by trimming "iB" and adding "B", which turns "MiB"
 * into "MB" and "MB" into "MBB". Every plain unit therefore matched no case
 * and was left unscaled: "659MB of 1.3GB" became 659 over 1 — the total being
 * an int64 cast of 1.3 — and the bar read 100% from the first few megabytes of
 * every download to the last.
 *
 * The letter is dropped from the middle instead, which is where it is: MiB,
 * GiB, KiB. Anything unrecognised is left unscaled, which is only right when
 * both sides of the comparison are unrecognised together — and they always
 * are, since one installer writes both.
 */
func bytesOf(value, unit string) int64 {
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}

	switch strings.ReplaceAll(strings.ToUpper(unit), "I", "") {
	case "GB":
		n *= 1 << 30
	case "MB":
		n *= 1 << 20
	case "KB":
		n *= 1 << 10
	}

	return int64(n)
}

// stripEscapes removes the terminal control codes progress bars are drawn with.
func stripEscapes(s string) string {
	var b strings.Builder

	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			b.WriteByte(s[i])

			continue
		}

		// Skip to the end of the escape sequence: a letter, or the string's end.
		for i++; i < len(s); i++ {
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				break
			}
		}
	}

	return b.String()
}
