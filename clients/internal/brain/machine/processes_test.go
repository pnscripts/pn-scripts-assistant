//go:build linux

package machine

import (
	"os"
	"testing"
)

/*
 * The three numbers, picked out of a real line by counting.
 *
 * Getting the count wrong is silent and total: the field beside utime is
 * stime, and the field beside the resident set is the resident *limit*, which
 * on most processes is eighteen million terabytes. Neither mistake fails —
 * both produce a number, and the panel shows it.
 *
 * This line is the real thing, from this machine, with a command name chosen
 * to break the naive parse: it contains spaces and a closing bracket.
 */
const oneProcess = `1234 (my app (x)) S 1 1234 1234 0 -1 4194560 12345 0 3 0 ` +
	`700 300 0 0 20 0 12 0 987654 1234567890 5000 18446744073709551615 ` +
	`94000000000000 94000000001000 140700000000000 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0`

func TestReadingTheNumbersOutOfAProcessLine(t *testing.T) {
	line := []byte(oneProcess)

	closed := lastIndexByte(line, ')')
	rest := line[closed+1:]

	/*
	 * utime is the fourteenth field overall and stime the fifteenth, which
	 * after the command are the twelfth and thirteenth of what is left. They
	 * are 700 and 300 in the line above, chosen to be different — the version
	 * before this read the same field for both and reported twice the user
	 * time as the total, with system time missing entirely.
	 */
	utime, ok := fieldNumber(rest, 11)
	if !ok || utime != 700 {
		t.Errorf("utime read as %v", utime)
	}

	stime, ok := fieldNumber(rest, 12)
	if !ok || stime != 300 {
		t.Errorf("stime read as %v; the field beside it is not the same field", stime)
	}

	/*
	 * The resident set is the twenty-fourth field, 5000 pages here. The field
	 * after it is the limit — 18446744073709551615 — and reading that one
	 * instead reports every process as using more memory than exists.
	 */
	rss, ok := fieldNumber(rest, 21)
	if !ok || rss != 5000 {
		t.Errorf("resident pages read as %v, want 5000", rss)
	}

	if limit, _ := fieldNumber(rest, 22); limit == 5000 {
		t.Error("the counting is off by one: the limit and the resident set read the same")
	}

	// And the name survives brackets and spaces inside it.
	open := indexByte(line, '(')

	if name := string(line[open+1 : closed]); name != "my app (x)" {
		t.Errorf("the command name read as %q", name)
	}
}

/*
 * And against this machine, where the answer can be checked against the
 * kernel's own arithmetic rather than against a line I wrote.
 */
func TestItAgreesWithTheKernelAboutThisProcess(t *testing.T) {
	me := os.Getpid()

	p, jiffies, ok := quickLook(me, 1<<34, float64(clockTicks()))
	if !ok {
		t.Fatal("could not read this very process")
	}

	if p.PID != me {
		t.Errorf("it read process %d", p.PID)
	}

	if jiffies < 0 {
		t.Errorf("processor time came back as %v", jiffies)
	}

	/*
	 * A running test process holds a few megabytes, and certainly less than
	 * the machine has. The resident *limit* is unlimited on most systems, so
	 * reading that field instead lands far outside this.
	 */
	if p.RSSBytes == 0 || p.RSSBytes > 8<<30 {
		t.Errorf("this process is using %d bytes of memory, which is not plausible", p.RSSBytes)
	}
}
